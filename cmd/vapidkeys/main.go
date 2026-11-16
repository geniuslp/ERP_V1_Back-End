// Command vapidkeys prints a new VAPID key pair for Web Push. Run it once per environment and put the two values
// in the environment / .env as VAPID_PUBLIC_KEY and VAPID_PRIVATE_KEY (plus VAPID_SUBJECT=mailto:...).
//
//	go run ./cmd/vapidkeys
package main

import (
	"fmt"
	"log"

	webpush "github.com/SherClockHolmes/webpush-go"
)

func main() {
	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("VAPID_PUBLIC_KEY=%s\nVAPID_PRIVATE_KEY=%s\n", pub, priv)
}
