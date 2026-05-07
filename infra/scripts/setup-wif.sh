#!/usr/bin/env bash
#
# Print the WIF provider + service account values to register as GitHub secrets.
# Run after applying infra/terraform/bootstrap.
#
# Required secrets (set in GitHub repo settings):
#   WIF_PROVIDER          → output `wif_provider`
#   WIF_SERVICE_ACCOUNT   → output `wif_service_account`

set -euo pipefail

cd "$(dirname "$0")/../terraform/bootstrap"

echo "Reading terraform outputs..."
echo
echo "WIF_PROVIDER:"
tofu output -raw wif_provider 2>/dev/null || terraform output -raw wif_provider
echo
echo
echo "WIF_SERVICE_ACCOUNT:"
tofu output -raw wif_service_account 2>/dev/null || terraform output -raw wif_service_account
echo
echo
echo "Set these as GitHub Actions secrets:"
echo "  https://github.com/titouanfreville/copro-manager/settings/secrets/actions"
