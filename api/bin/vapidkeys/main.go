// vapidkeys generates a VAPID keypair for Web Push.
//
// Usage:
//
//	cd api && go run ./bin/vapidkeys
//
// Output is two URL-safe base64 strings:
//
//	private: <paste into infra/terraform/env/terraform.tfvars vapid_private_key>
//	public:  <paste into vapid_public_key AND into the SvelteKit app's
//	         PUBLIC_VAPID_PUBLIC_KEY build env>
//
// The keypair is the long-lived identity the API uses to sign push
// payloads. Generate once, then keep both values out of git (the
// private key is gitignored via terraform.tfvars; the public key is
// fine to share but lives next to the private one for symmetry).
package main

import (
	"fmt"
	"os"

	webpush "github.com/SherClockHolmes/webpush-go"
)

func main() {
	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		fmt.Fprintf(os.Stderr, "vapidkeys: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("private:", priv)
	fmt.Println("public: ", pub)
}
