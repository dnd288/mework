package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"
)

// botRow is a decode target matching the BotResponse JSON.
type botRow struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	AppID       string `json:"app_id"`
	BaseURL     string `json:"base_url"`
	Status      string `json:"status"`
	Plan        string `json:"plan"`
	WorkspaceID string `json:"workspace_id"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

var mezonBotCmd = &cobra.Command{
	Use:   "bot",
	Short: "Manage Mezon bots on the server (turbo engine)",
}

var mezonBotRegisterCmd = &cobra.Command{
	Use:   "register",
	Short: "Register a new Mezon bot on the server",
	Long: `Register a Mezon bot with the server's turbo engine.

The bot is stored with credentials sealed at rest and registered
with the engine for real-time message processing.

Required: --app-id, --api-key
Optional: --name, --plan (starter|pro|enterprise), --base-url
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		appID, _ := cmd.Flags().GetString("app-id")
		apiKey, _ := cmd.Flags().GetString("api-key")
		name, _ := cmd.Flags().GetString("name")
		plan, _ := cmd.Flags().GetString("plan")
		baseURL, _ := cmd.Flags().GetString("base-url")

		if appID == "" || apiKey == "" {
			return fmt.Errorf("--app-id and --api-key are required")
		}
		if plan == "" {
			plan = "starter"
		}

		body := map[string]string{
			"app_id":   appID,
			"api_key":  apiKey,
			"name":     name,
			"plan":     plan,
			"base_url": baseURL,
		}

		var bot botRow
		status, err := mezonBotDo(http.MethodPost, "/api/v1/mezon/bots", body, &bot)
		if err != nil {
			return err
		}
		if status == http.StatusConflict {
			return fmt.Errorf("bot with app_id %q already exists", appID)
		}
		if status != http.StatusCreated {
			return fmt.Errorf("unexpected status %d", status)
		}

		fmt.Printf("Bot registered: %s (%s)\n", bot.ID, bot.Name)
		fmt.Printf("  App ID:  %s\n", bot.AppID)
		fmt.Printf("  Status:  %s\n", bot.Status)
		fmt.Printf("  Plan:    %s\n", bot.Plan)
		return nil
	},
}

var mezonBotListCmd = &cobra.Command{
	Use:   "list",
	Short: "List registered Mezon bots",
	RunE: func(cmd *cobra.Command, args []string) error {
		var bots []botRow
		status, err := mezonBotDo(http.MethodGet, "/api/v1/mezon/bots", nil, &bots)
		if err != nil {
			return err
		}
		if status != http.StatusOK {
			return fmt.Errorf("unexpected status %d", status)
		}

		if len(bots) == 0 {
			fmt.Println("No bots registered.")
			return nil
		}

		tw := newTableTo(cmd.OutOrStdout())
		row(tw, "ID", "NAME", "APP_ID", "STATUS", "PLAN")
		for _, b := range bots {
			row(tw, b.ID, b.Name, b.AppID, b.Status, b.Plan)
		}
		return tw.Flush()
	},
}

var mezonBotGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get details of a registered Mezon bot",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var bot botRow
		status, err := mezonBotDo(http.MethodGet, "/api/v1/mezon/bots/"+args[0], nil, &bot)
		if err != nil {
			return err
		}
		if status == http.StatusNotFound {
			return fmt.Errorf("bot %q not found", args[0])
		}
		if status != http.StatusOK {
			return fmt.Errorf("unexpected status %d", status)
		}

		fmt.Printf("ID:          %s\n", bot.ID)
		fmt.Printf("Name:        %s\n", bot.Name)
		fmt.Printf("App ID:      %s\n", bot.AppID)
		fmt.Printf("Base URL:    %s\n", bot.BaseURL)
		fmt.Printf("Status:      %s\n", bot.Status)
		fmt.Printf("Plan:        %s\n", bot.Plan)
		fmt.Printf("Workspace:   %s\n", bot.WorkspaceID)
		fmt.Printf("Created:     %s\n", bot.CreatedAt)
		fmt.Printf("Updated:     %s\n", bot.UpdatedAt)
		return nil
	},
}

var mezonBotRemoveCmd = &cobra.Command{
	Use:   "remove <id>",
	Short: "Deregister and remove a Mezon bot",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		status, err := mezonBotDo(http.MethodDelete, "/api/v1/mezon/bots/"+args[0], nil, nil)
		if err != nil {
			return err
		}
		if status == http.StatusNotFound {
			return fmt.Errorf("bot %q not found", args[0])
		}
		if status != http.StatusNoContent {
			return fmt.Errorf("unexpected status %d", status)
		}
		fmt.Println("Bot removed.")
		return nil
	},
}

var mezonBotActivateCmd = &cobra.Command{
	Use:   "activate <id>",
	Short: "Activate a Mezon bot (register with turbo engine)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		body := map[string]string{"status": "active"}
		status, err := mezonBotDo(http.MethodPatch, "/api/v1/mezon/bots/"+args[0]+"/status", body, nil)
		if err != nil {
			return err
		}
		if status == http.StatusNotFound {
			return fmt.Errorf("bot %q not found", args[0])
		}
		if status != http.StatusNoContent {
			return fmt.Errorf("unexpected status %d", status)
		}
		fmt.Println("Bot activated.")
		return nil
	},
}

var mezonBotDeactivateCmd = &cobra.Command{
	Use:   "deactivate <id>",
	Short: "Deactivate a Mezon bot (deregister from turbo engine)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		body := map[string]string{"status": "inactive"}
		status, err := mezonBotDo(http.MethodPatch, "/api/v1/mezon/bots/"+args[0]+"/status", body, nil)
		if err != nil {
			return err
		}
		if status == http.StatusNotFound {
			return fmt.Errorf("bot %q not found", args[0])
		}
		if status != http.StatusNoContent {
			return fmt.Errorf("unexpected status %d", status)
		}
		fmt.Println("Bot deactivated.")
		return nil
	},
}

func init() {
	mezonBotRegisterCmd.Flags().String("app-id", "", "Mezon app ID")
	mezonBotRegisterCmd.Flags().String("api-key", "", "Mezon API key")
	mezonBotRegisterCmd.Flags().String("name", "", "Human-readable name")
	mezonBotRegisterCmd.Flags().String("plan", "starter", "Plan tier: starter, pro, enterprise")
	mezonBotRegisterCmd.Flags().String("base-url", "", "Mezon API base URL")

	mezonBotCmd.AddCommand(mezonBotRegisterCmd, mezonBotListCmd, mezonBotGetCmd,
		mezonBotRemoveCmd, mezonBotActivateCmd, mezonBotDeactivateCmd)
	providerMezonCmd.AddCommand(mezonBotCmd)
}

// mezonBotDo sends an authenticated request to the server's bot API.
func mezonBotDo(method, path string, body, out interface{}) (int, error) {
	base, token, err := sessionEndpoint()
	if err != nil {
		return 0, err
	}

	var reqBody []byte
	if body != nil {
		reqBody, err = json.Marshal(body)
		if err != nil {
			return 0, fmt.Errorf("marshal: %w", err)
		}
	}

	req, err := http.NewRequest(method, base+path, bytes.NewReader(reqBody))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	if out != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp.StatusCode, nil
		}
	}

	return resp.StatusCode, nil
}
