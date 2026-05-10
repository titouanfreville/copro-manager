package gemini

import (
	"context"
	"encoding/json"
	"fmt"

	"google.golang.org/genai"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
)

// ReadMeterPhoto interprets a water-meter photo and returns the
// digit reading(s) Gemini extracted, plus a self-reported confidence.
//
// kind=global → 1 value (main building dial, m³ to 3 decimals).
// kind=detail → 3 values [common, rdc, premier], each m³ to 3 decimals.
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

	expected, err := expectedValueCount(kind)
	if err != nil {
		return nil, 0, err
	}
	prompt, err := promptForMeter(kind)
	if err != nil {
		return nil, 0, err
	}

	parts := []*genai.Part{
		{Text: prompt},
		{InlineData: &genai.Blob{Data: image, MIMEType: mimeType}},
	}
	cfg := &genai.GenerateContentConfig{
		ResponseMIMEType: "application/json",
		ResponseSchema:   meterSchema(expected),
		Temperature:      ptrF32(0),
	}
	resp, err := c.c.Models.GenerateContent(
		ctx,
		c.cfg.Model,
		[]*genai.Content{{Role: genai.RoleUser, Parts: parts}},
		cfg,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("gemini: generate content: %w", err)
	}
	c.recordCall(ctx)

	var parsed struct {
		Values     []float64 `json:"values"`
		Confidence float64   `json:"confidence"`
	}
	body := resp.Text()
	if body == "" {
		return nil, 0, fmt.Errorf("gemini: empty response")
	}
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		return nil, 0, fmt.Errorf("gemini: parse response: %w", err)
	}
	if len(parsed.Values) != expected {
		return nil, 0, fmt.Errorf("gemini: expected %d values, got %d", expected, len(parsed.Values))
	}
	if parsed.Confidence < 0 {
		parsed.Confidence = 0
	}
	if parsed.Confidence > 1 {
		parsed.Confidence = 1
	}
	return parsed.Values, parsed.Confidence, nil
}

// expectedValueCount returns the number of meter readings the given
// kind should produce. Used both as a contract assertion on the
// response and as MinItems/MaxItems for the response schema.
func expectedValueCount(kind entities.MeterPhotoKind) (int, error) {
	switch kind {
	case entities.MeterPhotoKindGlobal:
		return 1, nil
	case entities.MeterPhotoKindDetail:
		return 3, nil
	default:
		return 0, fmt.Errorf("gemini: unknown meter photo kind %q", kind)
	}
}

// promptForMeter returns the French prompt steering the model toward
// the correct extraction. Kept minimal — Gemini handles the visual
// reasoning (red drum decimals, dial vs serial labels, panel layout)
// natively; over-prompting hurts accuracy.
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

Renvoie {"values": [valeur_m3], "confidence": 0..1}.
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

Renvoie {"values": [commun, rdc, premier], "confidence": 0..1}, dans cet ordre exact (commun d'abord, puis RDC, puis 1er).
Si tu ne peux pas distinguer les 3 sous-compteurs avec certitude (compteur bleu non visible, positions ambiguës, photo trop floue pour compter les positions du tambour), mets confidence < 0.5.`, nil

	default:
		return "", fmt.Errorf("gemini: unknown meter photo kind %q", kind)
	}
}

// meterSchema builds the JSON schema constraining Gemini's response.
// Required count of values is fixed per kind (1 for global, 3 for
// detail) so the model can't return a partial reading.
func meterSchema(expectedValues int) *genai.Schema {
	count := int64(expectedValues)
	min0 := float64(0)
	max1 := float64(1)
	return &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"values": {
				Type:        genai.TypeArray,
				Description: "Lectures en m³, 3 décimales",
				Items:       &genai.Schema{Type: genai.TypeNumber},
				MinItems:    &count,
				MaxItems:    &count,
			},
			"confidence": {
				Type:        genai.TypeNumber,
				Description: "Auto-évaluation 0..1 de la lisibilité",
				Minimum:     &min0,
				Maximum:     &max1,
			},
		},
		Required: []string{"values", "confidence"},
	}
}

func ptrF32(v float32) *float32 { return &v }
