package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"mework/libs/shared/config"
)

var providerMezonCmd = &cobra.Command{
	Use:   "mezon",
	Short: "Configure Mezon provider and bot credentials",
}

var providerMezonSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Store Mezon bot credentials locally",
	Long: `Store Mezon bot credentials for the standalone mework-mezon-worker.

Credentials are saved to the active profile's config (~/.mework/config.json
or profile-specific) and used by 'mework mezon-worker start'.

Required: --app-id, --api-key
Optional: --base-url (default: https://api.mezon.vn)
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		appID := FlagOrEnv(cmd, "app-id", "MEZON_APP_ID", "")
		apiKey := FlagOrEnv(cmd, "api-key", "MEZON_API_KEY", "")
		baseURL := FlagOrEnv(cmd, "base-url", "MEZON_BASE_URL", "")

		if appID == "" {
			return fmt.Errorf("--app-id is required (or set MEZON_APP_ID)")
		}
		if apiKey == "" {
			return fmt.Errorf("--api-key is required (or set MEZON_API_KEY)")
		}

		prof := profile()
		cfg, err := config.LoadConfig(prof)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		cfg.Mezon = &config.MezonCredentials{
			AppID:   appID,
			APIKey:  apiKey,
			BaseURL: baseURL,
		}

		if err := cfg.Save(prof); err != nil {
			return fmt.Errorf("save config: %w", err)
		}

		fmt.Println("mezon credentials saved")
		return nil
	},
}

var providerMezonShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show stored Mezon credentials (masked)",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadConfig(profile())
		if err != nil {
			return err
		}
		if cfg.Mezon == nil {
			fmt.Println("no mezon credentials configured")
			fmt.Println("run: mework provider mezon set --app-id <id> --api-key <key>")
			return nil
		}

		maskedKey := cfg.Mezon.APIKey
		if len(maskedKey) > 8 {
			maskedKey = maskedKey[:4] + "****" + maskedKey[len(maskedKey)-4:]
		}

		fmt.Printf("App ID:  %s\n", cfg.Mezon.AppID)
		fmt.Printf("API Key: %s\n", maskedKey)
		if cfg.Mezon.BaseURL != "" {
			fmt.Printf("Base URL: %s\n", cfg.Mezon.BaseURL)
		} else {
			fmt.Println("Base URL: (default https://api.mezon.vn)")
		}
		return nil
	},
}

func init() {
	providerMezonSetCmd.Flags().String("app-id", "", "Mezon app ID")
	providerMezonSetCmd.Flags().String("api-key", "", "Mezon API key")
	providerMezonSetCmd.Flags().String("base-url", "", "Mezon API base URL (default: https://api.mezon.vn)")
	providerMezonCmd.AddCommand(providerMezonSetCmd, providerMezonShowCmd)
	providerCmd.AddCommand(providerMezonCmd)
}
