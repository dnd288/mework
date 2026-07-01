package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"mework/libs/shared/config"
	"mework/libs/shared/providers/mello"
)

var loginToken string

var loginCmd = &cobra.Command{
	Use:     "login",
	Short:   "Authenticate with a personal access token (Mello)",
	GroupID: groupAdditional,
	RunE: func(cmd *cobra.Command, args []string) error {
		token := loginToken
		// Empty --token (or omitted) → prompt, so the token avoids shell history.
		if token == "" {
			fmt.Print("API token: ")
			reader := bufio.NewReader(os.Stdin)
			line, _ := reader.ReadString('\n')
			token = strings.TrimSpace(line)
		}
		if token == "" {
			return fmt.Errorf("no token provided")
		}

		cfg, err := config.LoadConfig(profile())
		if err != nil {
			return err
		}
		// Validate before persisting.
		client := mello.NewClient(ResolveBaseURL(cmd, cfg), token, 0, version)
		user, err := client.GetCurrentUser()
		if err != nil {
			return err
		}
		cfg.Token = token
		if err := cfg.Save(profile()); err != nil {
			return err
		}
		fmt.Printf("Logged in as %s (%s)\n", user.Name, user.Email)
		return nil
	},
}

var authCmd = &cobra.Command{
	Use:     "auth",
	Short:   "Inspect or clear authentication",
	GroupID: groupAdditional,
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current authentication status",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, cfg, err := newRESTClient(cmd)
		if err != nil {
			return err
		}
		user, err := client.GetCurrentUser()
		if err != nil {
			return err
		}
		fmt.Printf("Server:  %s\n", ResolveBaseURL(cmd, cfg))
		fmt.Printf("User:    %s (%s)\n", user.Name, user.Email)
		fmt.Printf("Token:   %s\n", maskToken(ResolveToken(cfg)))
		return nil
	},
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Remove the stored token",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadConfig(profile())
		if err != nil {
			return err
		}
		cfg.Token = ""
		if err := cfg.Save(profile()); err != nil {
			return err
		}
		fmt.Println("Logged out.")
		return nil
	},
}

func init() {
	loginCmd.Flags().StringVar(&loginToken, "token", "", "Personal access token — omit to be prompted")
	authCmd.AddCommand(authStatusCmd, authLogoutCmd)
}
