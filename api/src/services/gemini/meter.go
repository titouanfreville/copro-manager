package gemini

import (
	"context"
	"encoding/json"
	"fmt"

	"google.golang.org/genai"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
)

// maxMeterValueM3 is the upper sanity bound on any single reading.
// Residential meters never exceed five digits to the left of the
// decimal in our experience; 10 000 000 m³ is well past any plausible
// real value and catches Gemini hallucinations that the schema's
// type-only constraint can't stop.
const maxMeterValueM3 = 10_000_000.0

// ReadMeterPhoto interprets a water-meter photo and returns the digit
// reading(s) Gemini extracted, plus a self-reported confidence.
//
// kind=global → 1 value (main building dial, m³ to 3 decimals).
// kind=detail → 3 values [common, rdc, premier], m³ to 3 decimals.
//
// The wire format Gemini produces is a LABELED object per kind
// (e.g. `{"common": x, "rdc": y, "premier": z, "confidence": c}` for
// detail) — never a positional array. This guards against prompt
// drift silently swapping RDC ↔ 1er readings: the JSON schema +
// unmarshal enforce the slot identity, and the slice we return is
// assembled Go-side in a canonical order.
//
// Implements interfaces.MeterReader.
func (c *Client) ReadMeterPhoto(
	ctx context.Context,
	kind entities.MeterPhotoKind,
	image []byte,
	mimeType string,
) ([]float64, float64, error) {
	if err := c.gate(ctx); err != nil {
		return nil, 0, err
	}
	if len(image) == 0 {
		return nil, 0, fmt.Errorf("gemini: empty image")
	}
	if mimeType == "" {
		mimeType = "image/jpeg"
	}

	prompt, err := promptForMeter(kind)
	if err != nil {
		return nil, 0, err
	}
	schema, err := meterSchemaFor(kind)
	if err != nil {
		return nil, 0, err
	}

	parts := []*genai.Part{
		{Text: prompt},
		{InlineData: &genai.Blob{Data: image, MIMEType: mimeType}},
	}
	cfg := &genai.GenerateContentConfig{
		ResponseMIMEType: "application/json",
		ResponseSchema:   schema,
		Temperature:      ptrF32(0),
	}
	resp, err := c.c.Models.GenerateContent(
		ctx,
		c.cfg.Model,
		[]*genai.Content{{Role: genai.RoleUser, Parts: parts}},
		cfg,
	)
	// Record the call regardless of post-call parsing success: the
	// Vertex AI bill is incurred as soon as GenerateContent returns,
	// so the counter must mirror that even when we fail to interpret
	// the response.
	if resp != nil || err == nil {
		c.recordCall(ctx)
	}
	if err != nil {
		return nil, 0, fmt.Errorf("gemini: generate content: %w", err)
	}

	body := resp.Text()
	if body == "" {
		return nil, 0, fmt.Errorf("gemini: empty response")
	}

	values, confidence, err := parseMeterResponse(kind, body)
	if err != nil {
		return nil, 0, err
	}
	for _, v := range values {
		if v < 0 || v > maxMeterValueM3 {
			return nil, 0, fmt.Errorf("gemini: value out of plausible range: %.3f", v)
		}
	}
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 1 {
		confidence = 1
	}
	return values, confidence, nil
}

// parseMeterResponse decodes Gemini's per-kind JSON into the canonical
// positional slice the usecase expects. Each kind has its own labeled
// shape; the unmarshal step enforces structural fit instead of relying
// on positional ordering.
//
// Pointer-typed numeric fields detect missing keys: Vertex AI's
// schema-Required enforcement is best-effort, and a missing `premier`
// would otherwise unmarshal as 0.0 and silently prefill the UI with
// zero for that slot. A nil pointer here surfaces as ErrAnalysisFailed
// so the frontend retries instead.
func parseMeterResponse(kind entities.MeterPhotoKind, body string) ([]float64, float64, error) {
	switch kind {
	case entities.MeterPhotoKindGlobal:
		var p struct {
			Value      *float64 `json:"value"`
			Confidence *float64 `json:"confidence"`
		}
		if err := json.Unmarshal([]byte(body), &p); err != nil {
			return nil, 0, fmt.Errorf("gemini: parse global response: %w", err)
		}
		if p.Value == nil || p.Confidence == nil {
			return nil, 0, fmt.Errorf("gemini: missing required field(s) in global response")
		}
		return []float64{*p.Value}, *p.Confidence, nil

	case entities.MeterPhotoKindDetail:
		var p struct {
			Common     *float64 `json:"common"`
			RDC        *float64 `json:"rdc"`
			Premier    *float64 `json:"premier"`
			Confidence *float64 `json:"confidence"`
		}
		if err := json.Unmarshal([]byte(body), &p); err != nil {
			return nil, 0, fmt.Errorf("gemini: parse detail response: %w", err)
		}
		if p.Common == nil || p.RDC == nil || p.Premier == nil || p.Confidence == nil {
			return nil, 0, fmt.Errorf("gemini: missing required field(s) in detail response")
		}
		// Canonical order matches the usecase + frontend contract:
		// [common, rdc, premier].
		return []float64{*p.Common, *p.RDC, *p.Premier}, *p.Confidence, nil

	default:
		return nil, 0, fmt.Errorf("gemini: unknown meter photo kind %q", kind)
	}
}

// promptForMeter returns the French prompt steering the model toward
// the correct extraction. The output shape is labeled per kind so the
// JSON parser can enforce slot identity (vs. a positional array which
// would silently swap RDC and 1er on prompt drift).
func promptForMeter(kind entities.MeterPhotoKind) (string, error) {
	switch kind {
	case entities.MeterPhotoKindGlobal:
		return `Photo du compteur d'eau général d'un immeuble résidentiel français.

ORIENTATION : ce compteur est installé physiquement à l'envers dans le mur. Selon la prise de vue, le cadran peut apparaître redressé (l'utilisateur a pivoté la photo avant l'envoi — cas habituel) OU retourné de 180°. Détecte l'orientation effective de l'image et lis les chiffres en conséquence — n'applique PAS de rotation mentale supplémentaire si les chiffres sont déjà droits.

FORMAT DE LECTURE (TRÈS IMPORTANT) :
Le cadran à chiffres est composé de DEUX tambours côte à côte :
- Le tambour de GAUCHE (fond blanc/noir) montre la partie ENTIÈRE — il a typiquement 4 à 7 positions de chiffres, à lire TOUTES, y compris les zéros initiaux.
- Le tambour de DROITE (fond ROUGE) montre les DÉCIMALES — il a EXACTEMENT 3 positions.
La valeur finale = (chiffres blancs).(chiffres rouges) en m³.

Exemples de format attendu :
- chiffres blancs "0252", chiffres rouges "735" → 252.735 m³
- chiffres blancs "0002527", chiffres rouges "355" → 2527.355 m³

NE SAUTE AUCUN CHIFFRE — vérifie que la valeur finale a bien le même nombre de positions que ce que tu vois sur le tambour. Si tu hésites entre deux longueurs (par ex. 4 vs 5 chiffres entiers), recompte les positions du tambour blanc avant de répondre.

Renvoie {"value": valeur_m3, "confidence": 0..1}.
Si la photo est floue, illisible, ou ne montre pas un compteur, mets confidence < 0.3.`, nil

	case entities.MeterPhotoKindDetail:
		return `Photo d'un panneau de 3 sous-compteurs d'eau dans un immeuble.

IDENTIFICATION DES COMPTEURS (les sous-compteurs ne sont PAS étiquetés) :
- Le compteur COMMUN est BLEU et d'un modèle visiblement différent des deux autres (forme, taille, marque).
- Le compteur 1er (premier étage) est le plus PROCHE du bleu, juste à côté.
- Le compteur RDC (rez-de-chaussée) est le plus ÉLOIGNÉ du bleu, au même niveau que le 1er.

FORMAT DE LECTURE (TRÈS IMPORTANT — Gemini se trompe régulièrement ici) :
Chaque cadran est composé de DEUX tambours côte à côte :
- Le tambour de GAUCHE (fond blanc/noir) montre la partie ENTIÈRE — typiquement 4 à 7 positions, à lire TOUTES, y compris les zéros initiaux.
- Le tambour de DROITE (fond ROUGE) montre les DÉCIMALES — EXACTEMENT 3 positions.
La valeur finale = (chiffres blancs).(chiffres rouges) en m³.

Exemples concrets vus sur ces compteurs :
- compteur BLEU (commun) : blancs "00074", rouges "525" → 74.525 m³
- compteur BLANC (RDC ou 1er) : blancs "0717", rouges "524" → 717.524 m³
- autre compteur BLANC : blancs "0731", rouges "442" → 731.442 m³

NE SAUTE AUCUN CHIFFRE. Erreur fréquente : lire "7452" au lieu de "74525" → tu obtiens 7.452 alors que la vraie valeur est 74.525. Compte précisément les positions du tambour blanc avant de répondre. Les sous-compteurs RDC et 1er ont typiquement 7 positions au total (4 blancs + 3 rouges), le commun bleu a typiquement 7 ou 8 positions.

Renvoie un objet ÉTIQUETÉ {"common": valeur_commun_bleu, "rdc": valeur_rdc, "premier": valeur_premier_etage, "confidence": 0..1}. Chaque clé identifie EXPLICITEMENT le sous-compteur — n'inverse pas RDC et 1er.
Si tu ne peux pas distinguer les 3 sous-compteurs avec certitude (compteur bleu non visible, positions ambiguës, photo trop floue pour compter les positions du tambour), mets confidence < 0.5.`, nil

	default:
		return "", fmt.Errorf("gemini: unknown meter photo kind %q", kind)
	}
}

// meterSchemaFor builds the JSON schema constraining Gemini's response
// per kind. Numeric items are bounded to [0, maxMeterValueM3] so the
// model can't return negative or wildly-implausible values; confidence
// is bounded to [0, 1].
func meterSchemaFor(kind entities.MeterPhotoKind) (*genai.Schema, error) {
	min0 := float64(0)
	maxV := float64(maxMeterValueM3)
	max1 := float64(1)
	confidenceField := &genai.Schema{
		Type:        genai.TypeNumber,
		Description: "Auto-évaluation 0..1 de la lisibilité",
		Minimum:     &min0,
		Maximum:     &max1,
	}
	valueField := func(label string) *genai.Schema {
		return &genai.Schema{
			Type:        genai.TypeNumber,
			Description: label + " en m³, 3 décimales",
			Minimum:     &min0,
			Maximum:     &maxV,
		}
	}

	switch kind {
	case entities.MeterPhotoKindGlobal:
		return &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"value":      valueField("Lecture du compteur général"),
				"confidence": confidenceField,
			},
			Required: []string{"value", "confidence"},
		}, nil

	case entities.MeterPhotoKindDetail:
		return &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"common":     valueField("Sous-compteur commun (bleu)"),
				"rdc":        valueField("Sous-compteur RDC"),
				"premier":    valueField("Sous-compteur 1er étage"),
				"confidence": confidenceField,
			},
			Required: []string{"common", "rdc", "premier", "confidence"},
		}, nil

	default:
		return nil, fmt.Errorf("gemini: unknown meter photo kind %q", kind)
	}
}

func ptrF32(v float32) *float32 { return &v }
