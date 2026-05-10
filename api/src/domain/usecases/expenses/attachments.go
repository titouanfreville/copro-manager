package expenses

// attachmentPrefix is the legacy GCS object-name prefix where per-expense
// attachments lived before the unified-documents migration. The Delete
// cascade still wipes this prefix as a defense-in-depth cleanup for any
// blob whose Document record was migrated and removed elsewhere — new
// uploads land at `documents/{docID}.{ext}` instead.
func attachmentPrefix(expenseID string) string {
	return "expenses/" + expenseID + "/"
}
