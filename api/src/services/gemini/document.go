package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"google.golang.org/genai"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	domainerrors "github.com/titouanfreville/copro-manager/api/src/domain/errors"
)

// AnalyzeDocument classifies a document image/PDF and extracts the
// per-kind structured fields the SvelteKit UI uses to pre-fill the
// expense / contract creation forms.
//
// On recoverable failures (empty response, malformed JSON, parse
// errors) returns `domainerrors.ErrAnalysisFailed` wrapped with the
// underlying cause. The route maps that to a 422 the UI can render as
// "réessaie" instead of a 500. Infra failures (gating, Vertex network
// error) keep their original error so the existing FEATURE_* and
// 5xx mappings still fire.
//
// Implements interfaces.DocumentAnalyzer.
func (c *Client) AnalyzeDocument(
	ctx context.Context,
	image []byte,
	mimeType string,
) (*entities.DocumentAnalysis, error) {
	if err := c.gate(ctx); err != nil {
		return nil, err
	}
	if len(image) == 0 {
		return nil, fmt.Errorf("gemini: empty document")
	}
	if mimeType == "" {
		mimeType = "image/jpeg"
	}

	parts := []*genai.Part{
		{Text: documentPrompt},
		{InlineData: &genai.Blob{Data: image, MIMEType: mimeType}},
	}
	cfg := &genai.GenerateContentConfig{
		ResponseMIMEType: "application/json",
		ResponseSchema:   documentSchema(),
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
		return nil, fmt.Errorf("gemini: generate content: %w", err)
	}

	body := resp.Text()
	if body == "" {
		return nil, fmt.Errorf("%w: empty response", domainerrors.ErrAnalysisFailed)
	}

	var parsed documentResponse
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		return nil, fmt.Errorf("%w: parse response: %v", domainerrors.ErrAnalysisFailed, err)
	}
	return parsed.toEntity(c.cfg.Model), nil
}

// documentResponse mirrors the JSON shape Gemini is constrained to
// produce by documentSchema. Lives next to the schema definition so
// changes stay in sync.
type documentResponse struct {
	Kind       string  `json:"kind"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason,omitempty"`
	Expense    *struct {
		AmountEUR    float64 `json:"amount_eur,omitempty"`
		Date         string  `json:"date,omitempty"`
		Vendor       string  `json:"vendor,omitempty"`
		CategoryHint string  `json:"category_hint,omitempty"`
		Description  string  `json:"description,omitempty"`
	} `json:"expense,omitempty"`
	Contract *struct {
		Provider         string  `json:"provider,omitempty"`
		ContractType     string  `json:"contract_type,omitempty"`
		StartDate        string  `json:"start_date,omitempty"`
		EndDate          string  `json:"end_date,omitempty"`
		MonthlyAmountEUR float64 `json:"monthly_amount_eur,omitempty"`
		ContractNumber   string  `json:"contract_number,omitempty"`
	} `json:"contract,omitempty"`
}

// maxDocumentAmountEUR caps any single monetary value extracted from
// a document. €1 000 000 is well past any realistic invoice/contract
// the copro will ever see; catches Gemini hallucinations that the
// schema's type-only constraint can't stop and prevents nonsense
// values from pre-filling the UI form.
const maxDocumentAmountEUR = 1_000_000.0

func (r documentResponse) toEntity(model string) *entities.DocumentAnalysis {
	kind := entities.DocumentAnalysisKind(r.Kind)
	// Defensive: the schema enum should prevent unknown values. If
	// Gemini drifts AND populates an extraction block, pick whichever
	// block has more populated fields rather than guessing — only
	// fall back to "other" when neither is meaningfully populated.
	if !entities.IsKnownDocumentAnalysisKind(kind) {
		expenseScore := scoreExpensePayload(r.Expense)
		contractScore := scoreContractPayload(r.Contract)
		switch {
		case expenseScore == 0 && contractScore == 0:
			kind = entities.DocumentKindOther
		case expenseScore >= contractScore:
			kind = entities.DocumentKindExpense
		default:
			kind = entities.DocumentKindContract
		}
	}
	conf := r.Confidence
	if conf < 0 {
		conf = 0
	}
	if conf > 1 {
		conf = 1
	}
	out := &entities.DocumentAnalysis{
		Kind:       kind,
		Confidence: conf,
		AnalyzedAt: time.Now().UTC(),
		Model:      model,
		Reason:     r.Reason,
	}
	if kind == entities.DocumentKindExpense && r.Expense != nil {
		out.Expense = &entities.ExpenseExtraction{
			AmountEUR:    clampAmount(r.Expense.AmountEUR),
			Date:         r.Expense.Date,
			Vendor:       r.Expense.Vendor,
			CategoryHint: r.Expense.CategoryHint,
			Description:  r.Expense.Description,
		}
	}
	if kind == entities.DocumentKindContract && r.Contract != nil {
		out.Contract = &entities.ContractExtraction{
			Provider:         r.Contract.Provider,
			ContractType:     r.Contract.ContractType,
			StartDate:        r.Contract.StartDate,
			EndDate:          r.Contract.EndDate,
			MonthlyAmountEUR: clampAmount(r.Contract.MonthlyAmountEUR),
			ContractNumber:   r.Contract.ContractNumber,
		}
	}
	return out
}

// clampAmount returns 0 when the model returned a negative or wildly-
// out-of-range value; otherwise returns it unchanged. Zero is a
// reasonable "missing" sentinel for the UI (the user will see an
// empty amount field rather than nonsense).
func clampAmount(v float64) float64 {
	if v < 0 || v > maxDocumentAmountEUR {
		return 0
	}
	return v
}

// scoreExpensePayload counts how many extraction fields the model
// populated. Used as a tie-breaker when `kind` drifts and we have to
// guess which block was meaningful.
func scoreExpensePayload(e *struct {
	AmountEUR    float64 `json:"amount_eur,omitempty"`
	Date         string  `json:"date,omitempty"`
	Vendor       string  `json:"vendor,omitempty"`
	CategoryHint string  `json:"category_hint,omitempty"`
	Description  string  `json:"description,omitempty"`
}) int {
	if e == nil {
		return 0
	}
	n := 0
	if e.AmountEUR > 0 {
		n++
	}
	if e.Date != "" {
		n++
	}
	if e.Vendor != "" {
		n++
	}
	if e.CategoryHint != "" {
		n++
	}
	if e.Description != "" {
		n++
	}
	return n
}

func scoreContractPayload(c *struct {
	Provider         string  `json:"provider,omitempty"`
	ContractType     string  `json:"contract_type,omitempty"`
	StartDate        string  `json:"start_date,omitempty"`
	EndDate          string  `json:"end_date,omitempty"`
	MonthlyAmountEUR float64 `json:"monthly_amount_eur,omitempty"`
	ContractNumber   string  `json:"contract_number,omitempty"`
}) int {
	if c == nil {
		return 0
	}
	n := 0
	if c.Provider != "" {
		n++
	}
	if c.ContractType != "" {
		n++
	}
	if c.StartDate != "" {
		n++
	}
	if c.EndDate != "" {
		n++
	}
	if c.MonthlyAmountEUR > 0 {
		n++
	}
	if c.ContractNumber != "" {
		n++
	}
	return n
}

const documentPrompt = `Analyse ce document scanné ou photographié (PDF ou image).

OBJECTIF : déterminer si c'est :
- "expense"  → un reçu / une facture / un ticket de caisse / une note ponctuelle à payer
- "contract" → un contrat / une police d'assurance / un mandat de syndic / un engagement récurrent ou pluriannuel
- "other"    → autre chose (procès-verbal d'AG, devis non signé, rapport technique, courrier divers, …)

POUR UN EXPENSE, extrais si visibles :
- amount_eur : le montant total TTC (ou TOTAL à payer) en euros, nombre décimal (par ex. 127.50). Si plusieurs montants apparaissent, choisis le total final, pas un sous-total.
- date : la date d'émission ou de la transaction, au format ISO YYYY-MM-DD.
- vendor : le nom du commerçant / fournisseur (raison sociale ou enseigne).
- category_hint : un mot-clé court suggérant la catégorie (par ex. "eau", "électricité", "assurance", "travaux", "fournitures"). Laisse vide si tu ne sais pas.
- description : un résumé d'une phrase ("Facture EDF mars 2026", "Réparation chaudière").

POUR UN CONTRACT, extrais si visibles :
- provider : le prestataire (assureur, syndic, fournisseur d'énergie).
- contract_type : par ex. "assurance habitation", "syndic", "électricité", "gaz", "internet".
- start_date / end_date : ISO YYYY-MM-DD.
- monthly_amount_eur : la mensualité ou (annuel / 12).
- contract_number : la référence du contrat / numéro de police.

CONFIDENCE :
- 1.0 → document clair, classification évidente, tous les champs lisibles.
- 0.5-0.8 → classification confiante mais certains champs incertains / illisibles.
- < 0.5 → classification incertaine ou document trop dégradé. Mets "other" avec un Reason si tu n'arrives pas à classer.

LANGUE : les documents sont en français. Les montants utilisent la virgule décimale (1 234,56 €) — convertis-les en notation à point pour le JSON (1234.56).

NE JAMAIS halluciner un champ. Laisse un champ vide si tu ne peux pas le lire avec certitude. Mieux vaut un champ manquant qu'une valeur inventée.`

// documentSchema constrains Gemini's response to the discriminated-
// union shape (top-level kind+confidence, plus a per-kind nested
// extraction object). Vertex AI doesn't enforce conditional required
// fields based on the discriminator, so each nested object's fields
// are individually optional and Gemini fills the relevant one per the
// prompt.
func documentSchema() *genai.Schema {
	min0 := float64(0)
	max1 := float64(1)
	return &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"kind": {
				Type:        genai.TypeString,
				Enum:        []string{"expense", "contract", "other"},
				Description: "expense=reçu/facture; contract=engagement récurrent; other=autre",
			},
			"confidence": {
				Type:    genai.TypeNumber,
				Minimum: &min0,
				Maximum: &max1,
			},
			"reason": {
				Type:        genai.TypeString,
				Description: "Justification courte (surtout utile pour kind=other)",
			},
			"expense": {
				Type:        genai.TypeObject,
				Description: "Rempli uniquement quand kind=expense",
				Properties: map[string]*genai.Schema{
					"amount_eur":    {Type: genai.TypeNumber, Description: "Total TTC en euros"},
					"date":          {Type: genai.TypeString, Description: "ISO YYYY-MM-DD"},
					"vendor":        {Type: genai.TypeString},
					"category_hint": {Type: genai.TypeString},
					"description":   {Type: genai.TypeString},
				},
			},
			"contract": {
				Type:        genai.TypeObject,
				Description: "Rempli uniquement quand kind=contract",
				Properties: map[string]*genai.Schema{
					"provider":           {Type: genai.TypeString},
					"contract_type":      {Type: genai.TypeString},
					"start_date":         {Type: genai.TypeString, Description: "ISO YYYY-MM-DD"},
					"end_date":           {Type: genai.TypeString, Description: "ISO YYYY-MM-DD"},
					"monthly_amount_eur": {Type: genai.TypeNumber},
					"contract_number":    {Type: genai.TypeString},
				},
			},
		},
		Required: []string{"kind", "confidence"},
	}
}
