// Seed is a one-shot CLI that provisions the singleton Copro and the two
// foyer accounts (Firebase Auth user + users doc + foyers doc with the
// initial member). Idempotent: re-running reuses existing Firebase Auth
// users, the existing Copro, and never overwrites a foyer's Parts.
//
// Usage:
//
//	go run ./bin/seed \
//	  --rdc-email titouan+rdc@example.com --rdc-name "Foyer RDC" \
//	  --1er-email titouan+1er@example.com --1er-name "Foyer 1er"
//
// The Firebase project ID is read from conf/main.yml (firestore.project_id).
// Credentials come from Application Default Credentials — locally, run
// `gcloud auth application-default login` first.
//
// Local emulator mode: set FIREBASE_AUTH_EMULATOR_HOST and FIRESTORE_EMULATOR_HOST
// before running, and the SDKs auto-detect them.
//
// The admin UI exposes the same flows; this CLI exists for first-time
// bootstrap before Firebase Hosting is up.
package main

import (
	"context"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/big"
	"os"
	"strings"

	fs "cloud.google.com/go/firestore"
	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
	"github.com/google/uuid"
	"google.golang.org/api/iterator"

	"github.com/titouanfreville/copro-manager/api/src/core/config"
	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
)

const (
	coprosCollection = "copros"
	foyersCollection = "foyers"
	usersCollection  = "users"

	pwdLowers   = "abcdefghijkmnopqrstuvwxyz"
	pwdUppers   = "ABCDEFGHJKLMNPQRSTUVWXYZ"
	pwdDigits   = "23456789"
	pwdSpecials = "!@#$%^&*-_+="
	pwdLength   = 16
)

const defaultPartsPerFoyer = entities.DefaultTotalParts / 2

type foyerSeed struct {
	floor entities.FoyerFloor
	email string
	name  string
}

type coproDoc struct {
	ID         string `firestore:"id"`
	Name       string `firestore:"name"`
	Address    string `firestore:"address"`
	TotalParts int    `firestore:"total_parts"`
}

type foyerDoc struct {
	ID        string              `firestore:"id"`
	CoproID   string              `firestore:"copro_id"`
	Floor     entities.FoyerFloor `firestore:"floor"`
	Name      string              `firestore:"name"`
	MemberIDs []string            `firestore:"member_ids"`
	Parts     int                 `firestore:"parts"`
}

type userDoc struct {
	ID          string `firestore:"id"`
	Email       string `firestore:"email"`
	DisplayName string `firestore:"display_name"`
}

func main() {
	rdcEmail := flag.String("rdc-email", "", "Email for the RDC foyer (required)")
	rdcName := flag.String("rdc-name", "Foyer RDC", "Display name for the RDC foyer")
	premierEmail := flag.String("1er-email", "", "Email for the 1er foyer (required)")
	premierName := flag.String("1er-name", "Foyer 1er", "Display name for the 1er foyer")
	configFile := flag.String("config", "conf/main.yml", "Path(s) to the YAML config file(s), separated by ':'")

	flag.Parse()

	if *rdcEmail == "" || *premierEmail == "" {
		fmt.Fprintln(os.Stderr, "error: --rdc-email and --1er-email are required")
		flag.Usage()
		os.Exit(2)
	}

	cfg := config.NewConfigFromYAML(strings.Split(*configFile, string(os.PathListSeparator))...)
	projectID := cfg.Firestore.ProjectID
	if projectID == "" {
		log.Fatal("firestore.project_id is empty in config")
	}

	ctx := context.Background()

	app, err := firebase.NewApp(ctx, &firebase.Config{ProjectID: projectID})
	if err != nil {
		log.Fatalf("firebase init: %v", err)
	}

	authClient, err := app.Auth(ctx)
	if err != nil {
		log.Fatalf("firebase auth: %v", err)
	}

	var fsClient *fs.Client
	if cfg.Firestore.Database == "" || cfg.Firestore.Database == "(default)" {
		fsClient, err = fs.NewClient(ctx, projectID)
	} else {
		fsClient, err = fs.NewClientWithDatabase(ctx, projectID, cfg.Firestore.Database)
	}
	if err != nil {
		log.Fatalf("firestore init: %v", err)
	}
	defer func() { _ = fsClient.Close() }()

	fmt.Printf("Seeding copro-manager in project %q (database %q)\n\n", projectID, defaultDatabase(cfg.Firestore.Database))

	copro, coproCreated, err := upsertCopro(ctx, fsClient)
	if err != nil {
		log.Fatalf("seed copro: %v", err)
	}

	if coproCreated {
		fmt.Printf("✓ Created copro id=%s total_parts=%d (defaults: name=%q, address=\"\")\n", copro.ID, copro.TotalParts, copro.Name)
	} else {
		fmt.Printf("✓ Reused copro  id=%s total_parts=%d\n", copro.ID, copro.TotalParts)
	}
	fmt.Println()

	seeds := []foyerSeed{
		{floor: entities.FoyerFloorRDC, email: *rdcEmail, name: *rdcName},
		{floor: entities.FoyerFloor1er, email: *premierEmail, name: *premierName},
	}

	for _, s := range seeds {
		if err := seedFoyer(ctx, authClient, fsClient, copro.ID, s); err != nil {
			log.Fatalf("seed floor=%s: %v", s.floor, err)
		}
	}

	fmt.Println("\nDone. Copro and both foyers are ready.")
}

func defaultDatabase(d string) string {
	if d == "" {
		return "(default)"
	}
	return d
}

func upsertCopro(ctx context.Context, fsClient *fs.Client) (*coproDoc, bool, error) {
	iter := fsClient.Collection(coprosCollection).Limit(1).Documents(ctx)
	defer iter.Stop()

	snap, err := iter.Next()
	if err == nil {
		var doc coproDoc
		if err := snap.DataTo(&doc); err != nil {
			return nil, false, err
		}
		return &doc, false, nil
	}

	if !errors.Is(err, iterator.Done) {
		return nil, false, err
	}

	doc := coproDoc{
		ID:         uuid.NewString(),
		Name:       "Copro",
		Address:    "",
		TotalParts: entities.DefaultTotalParts,
	}

	if _, err := fsClient.Collection(coprosCollection).Doc(doc.ID).Create(ctx, doc); err != nil {
		return nil, false, err
	}

	return &doc, true, nil
}

func seedFoyer(ctx context.Context, authClient *auth.Client, fsClient *fs.Client, coproID string, s foyerSeed) error {
	authUser, authCreated, err := upsertFirebaseUser(ctx, authClient, s.email, s.name)
	if err != nil {
		return fmt.Errorf("firebase user: %w", err)
	}
	if authCreated {
		fmt.Printf("✓ Created auth   %-30s  uid=%s\n", s.email, authUser.UID)
	} else {
		fmt.Printf("✓ Reused auth    %-30s  uid=%s\n", s.email, authUser.UID)
	}

	if err := upsertUserDoc(ctx, fsClient, authUser.UID, s.email, s.name); err != nil {
		return fmt.Errorf("user doc: %w", err)
	}

	id, docCreated, err := upsertFoyerDoc(ctx, fsClient, coproID, s, authUser.UID)
	if err != nil {
		return fmt.Errorf("foyer doc: %w", err)
	}
	if docCreated {
		fmt.Printf("  Created foyer doc floor=%-3s  id=%s  parts=%d\n", s.floor, id, defaultPartsPerFoyer)
	} else {
		fmt.Printf("  Updated foyer doc floor=%-3s  id=%s  (parts preserved)\n", s.floor, id)
	}

	return nil
}

func upsertFirebaseUser(ctx context.Context, c *auth.Client, email, name string) (*auth.UserRecord, bool, error) {
	user, err := c.GetUserByEmail(ctx, email)
	if err == nil {
		return user, false, nil
	}

	if !auth.IsUserNotFound(err) {
		return nil, false, err
	}

	password, err := generatePassword()
	if err != nil {
		return nil, false, fmt.Errorf("generate password: %w", err)
	}

	user, err = c.CreateUser(ctx, (&auth.UserToCreate{}).
		Email(email).
		EmailVerified(true).
		Password(password).
		DisplayName(name).
		Disabled(false))
	if err != nil {
		return nil, false, err
	}

	fmt.Printf("\n  ⚠ Initial password for %s — save it now, it is not stored:\n      %s\n\n", email, password)

	return user, true, nil
}

func upsertUserDoc(ctx context.Context, fsClient *fs.Client, uid, email, name string) error {
	doc := userDoc{ID: uid, Email: email, DisplayName: name}
	_, err := fsClient.Collection(usersCollection).Doc(uid).Set(ctx, doc)
	return err
}

func upsertFoyerDoc(ctx context.Context, fsClient *fs.Client, coproID string, s foyerSeed, uid string) (string, bool, error) {
	existing, err := findFoyerByFloor(ctx, fsClient, s.floor)
	if err != nil {
		return "", false, err
	}

	if existing != nil {
		members := mergeMember(existing.MemberIDs, uid)
		updates := []fs.Update{
			{Path: "name", Value: s.name},
			{Path: "member_ids", Value: members},
		}
		if _, err := fsClient.Collection(foyersCollection).Doc(existing.ID).Update(ctx, updates); err != nil {
			return "", false, err
		}
		return existing.ID, false, nil
	}

	id := uuid.NewString()
	doc := foyerDoc{
		ID:        id,
		CoproID:   coproID,
		Floor:     s.floor,
		Name:      s.name,
		MemberIDs: []string{uid},
		Parts:     defaultPartsPerFoyer,
	}

	if _, err := fsClient.Collection(foyersCollection).Doc(id).Create(ctx, doc); err != nil {
		return "", false, err
	}

	return id, true, nil
}

func findFoyerByFloor(ctx context.Context, fsClient *fs.Client, floor entities.FoyerFloor) (*foyerDoc, error) {
	iter := fsClient.Collection(foyersCollection).Where("floor", "==", string(floor)).Limit(1).Documents(ctx)
	defer iter.Stop()

	snap, err := iter.Next()
	if errors.Is(err, iterator.Done) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var doc foyerDoc
	if err := snap.DataTo(&doc); err != nil {
		return nil, err
	}

	return &doc, nil
}

func mergeMember(existing []string, uid string) []string {
	for _, m := range existing {
		if m == uid {
			return existing
		}
	}
	return append(existing, uid)
}

// generatePassword targets Firebase's strictest policy: at least one of each
// character class. See the matching generator in src/adapters/auth/firebase.go.
func generatePassword() (string, error) {
	classes := []string{pwdLowers, pwdUppers, pwdDigits, pwdSpecials}
	pwd := make([]byte, 0, pwdLength)
	for _, c := range classes {
		ch, err := randChar(c)
		if err != nil {
			return "", err
		}
		pwd = append(pwd, ch)
	}

	all := pwdLowers + pwdUppers + pwdDigits + pwdSpecials
	for len(pwd) < pwdLength {
		ch, err := randChar(all)
		if err != nil {
			return "", err
		}
		pwd = append(pwd, ch)
	}

	if err := cryptoShuffle(pwd); err != nil {
		return "", err
	}
	return string(pwd), nil
}

func randChar(set string) (byte, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(set))))
	if err != nil {
		return 0, fmt.Errorf("random char: %w", err)
	}
	return set[n.Int64()], nil
}

func cryptoShuffle(b []byte) error {
	for i := len(b) - 1; i > 0; i-- {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return fmt.Errorf("shuffle: %w", err)
		}
		j := n.Int64()
		b[i], b[j] = b[j], b[i]
	}
	return nil
}
