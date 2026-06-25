package dns

import (
	"bufio"
	"fmt"
	"os"
)

// ManualProvider prints the CNAME record the user needs to create and waits
// for them to confirm before continuing setup.
type ManualProvider struct{}

func (p *ManualProvider) Name() string { return "Manual" }

func (p *ManualProvider) SetupWildcard(domain, target string) error {
	fmt.Println()
	fmt.Println("  Add this DNS record in your provider's dashboard:")
	fmt.Println()
	fmt.Printf("    Type:   CNAME\n")
	fmt.Printf("    Name:   *.%s  (or just * if the UI strips the domain)\n", domain)
	fmt.Printf("    Value:  %s\n", target)
	fmt.Printf("    TTL:    600\n")
	fmt.Println()
	fmt.Print("  Press Enter once the record is saved... ")

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	return nil
}
