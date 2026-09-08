package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"log"
	"os"

	"crowns/internal/httpapi"
)

func main() {
	check := flag.Bool("check", false, "Fail if generated contracts differ")
	flag.Parse()
	data, err := json.MarshalIndent(httpapi.OpenAPI(), "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	files := map[string][]byte{"api/openapi.json": append(data, '\n'), "web/src/api.generated.ts": []byte(httpapi.TypeScript())}
	for path, data := range files {
		if *check {
			existing, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(existing, data) {
				log.Fatalf("%s is stale; run go run ./cmd/contracts", path)
			}
			continue
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			log.Fatal(err)
		}
	}
}
