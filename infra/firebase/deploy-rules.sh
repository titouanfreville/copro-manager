#!/usr/bin/env bash
# Deploy Firestore security rules and indexes to the copro-manager GCP project.
#
# Pre-requisites:
#   - firebase-tools installed (`npm i -g firebase-tools`)
#   - logged in (`firebase login`) with an account that has the
#     `roles/firebaserules.admin` IAM role on copro-494909.
#
# Usage: ./deploy-rules.sh

set -euo pipefail

PROJECT_ID="${FIREBASE_PROJECT_ID:-copro-494909}"

cd "$(dirname "$0")"

echo "Deploying Firestore rules + indexes to project ${PROJECT_ID}…"
firebase deploy --only firestore:rules,firestore:indexes --project "${PROJECT_ID}"
echo "✓ Rules + indexes deployed."
