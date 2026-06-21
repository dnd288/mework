package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var (
	providerName  string
	providerToken string
	webhookSecret string
)

var providerCmd = &cobra.Command{
	Use:   "provider",
	Short: "Manage third-party providers",
}

var providerConnectCmd = &cobra.Command{
	Use:   "connect",
	Short: "Connect a third-party provider account",
	RunE: func(cmd *cobra.Command, args []string) error {
		if providerName == "" {
			providerName = "mello" // Default to mello
		}

		token := providerToken
		if token == "" {
			fmt.Print("Provider personal access token: ")
			reader := bufio.NewReader(os.Stdin)
			line, _ := reader.ReadString('\n')
			token = strings.TrimSpace(line)
		}

		if token == "" {
			return fmt.Errorf("provider token is required")
		}

		mewClient, cfg, err := newMeworkClient()
		if err != nil {
			return err
		}

		patToken := cfg.Token
		if patToken == "" {
			return fmt.Errorf("not authenticated — run `mework login` first")
		}

		conn, err := mewClient.CreateConnection(patToken, providerName, token, webhookSecret, nil)
		if err != nil {
			return err
		}

		fmt.Printf("Connected provider %q successfully. Connection ID: %s\n", conn.ProviderCode, conn.ID)
		return nil
	},
}

func init() {
	providerConnectCmd.Flags().StringVar(&providerName, "provider", "mello", "Provider code (default: mello)")
	providerConnectCmd.Flags().StringVar(&providerToken, "token", "", "Provider personal access token (omit to prompt)")
	providerConnectCmd.Flags().StringVar(&webhookSecret, "webhook-secret", "", "Webhook signing secret (optional)")
	providerCmd.AddCommand(providerConnectCmd)
}
