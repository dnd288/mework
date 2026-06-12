package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var ticketCmd = &cobra.Command{
	Use:     "ticket",
	Aliases: []string{"t"},
	Short:   "Manage tickets",
	GroupID: groupCore,
}

var ticketListCmd = &cobra.Command{
	Use:   "list <board-id>",
	Short: "List tickets on a board",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _, err := newRESTClient(cmd)
		if err != nil {
			return err
		}
		tickets, err := client.ListBoardTickets(args[0])
		if err != nil {
			return err
		}
		if jsonFlag {
			return printJSON(tickets)
		}
		w := newTable()
		row(w, "CODE", "TITLE", "ASSIGNEE")
		for _, t := range tickets {
			row(w, t.TicketCode, t.Title, t.AssigneeID)
		}
		return w.Flush()
	},
}

var ticketGetCmd = &cobra.Command{
	Use:   "get <ticket-id>",
	Short: "Get a ticket with comments and checklists",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _, err := newRESTClient(cmd)
		if err != nil {
			return err
		}
		t, err := client.GetTicket(args[0])
		if err != nil {
			return err
		}
		if jsonFlag {
			return printJSON(t)
		}
		fmt.Printf("%s  %s\n%s\n\n%d comment(s), %d checklist item(s)\n",
			t.TicketCode, t.Title, t.Description, len(t.Comments), t.ChecklistItemCount)
		return nil
	},
}

var ticketCreateColumn, ticketCreateTitle, ticketCreateDesc string

var ticketCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a ticket in a column",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _, err := newRESTClient(cmd)
		if err != nil {
			return err
		}
		// create_ticket isn't in the read methods; use UpdateTicket-style raw call.
		updates := map[string]any{"title": ticketCreateTitle}
		if ticketCreateDesc != "" {
			updates["description"] = ticketCreateDesc
		}
		created, err := client.CreateTicket(ticketCreateColumn, updates)
		if err != nil {
			return err
		}
		fmt.Printf("Created %s (%s)\n", created.TicketCode, created.ID)
		return nil
	},
}

var ticketMoveColumn string
var ticketMovePosition int

var ticketMoveCmd = &cobra.Command{
	Use:   "move <ticket-id>",
	Short: "Move a ticket to a column/position",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _, err := newRESTClient(cmd)
		if err != nil {
			return err
		}
		res, err := client.MoveTicket(args[0], ticketMoveColumn, ticketMovePosition)
		if err != nil {
			return err
		}
		fmt.Printf("Moved %s: %s → %s\n", res.Ticket.TicketCode, res.FromColumn, res.ToColumn)
		return nil
	},
}

var commentCmd = &cobra.Command{
	Use:     "comment",
	Short:   "Manage ticket comments",
	GroupID: groupCore,
}

var commentListCmd = &cobra.Command{
	Use:   "list <ticket-id>",
	Short: "List comments on a ticket",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _, err := newRESTClient(cmd)
		if err != nil {
			return err
		}
		comments, err := client.ListComments(args[0])
		if err != nil {
			return err
		}
		if jsonFlag {
			return printJSON(comments)
		}
		for _, c := range comments {
			fmt.Printf("- [%s] %s\n", c.UserID, c.Body)
		}
		return nil
	},
}

var commentAddBody string

var commentAddCmd = &cobra.Command{
	Use:   "add <ticket-id>",
	Short: "Add a comment to a ticket",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _, err := newRESTClient(cmd)
		if err != nil {
			return err
		}
		c, err := client.CreateComment(args[0], commentAddBody)
		if err != nil {
			return err
		}
		fmt.Printf("Added comment %s\n", c.ID)
		return nil
	},
}

var searchCmd = &cobra.Command{
	Use:     "search <query>",
	Short:   "Search tickets in the workspace",
	GroupID: groupCore,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, cfg, err := newRESTClient(cmd)
		if err != nil {
			return err
		}
		ws, err := requireWorkspaceID(cmd, cfg)
		if err != nil {
			return err
		}
		results, err := client.SearchTickets(ws, args[0])
		if err != nil {
			return err
		}
		if jsonFlag {
			return printJSON(results)
		}
		w := newTable()
		row(w, "CODE", "TITLE", "BOARD")
		for _, r := range results {
			row(w, r.TicketCode, r.Title, r.BoardName)
		}
		return w.Flush()
	},
}

func init() {
	ticketListCmd.Flags().BoolVar(&jsonFlag, "json", false, "output as JSON")
	ticketGetCmd.Flags().BoolVar(&jsonFlag, "json", false, "output as JSON")
	commentListCmd.Flags().BoolVar(&jsonFlag, "json", false, "output as JSON")
	searchCmd.Flags().BoolVar(&jsonFlag, "json", false, "output as JSON")

	ticketCreateCmd.Flags().StringVar(&ticketCreateColumn, "column", "", "column id (required)")
	ticketCreateCmd.Flags().StringVar(&ticketCreateTitle, "title", "", "ticket title (required)")
	ticketCreateCmd.Flags().StringVar(&ticketCreateDesc, "description", "", "ticket description")
	_ = ticketCreateCmd.MarkFlagRequired("column")
	_ = ticketCreateCmd.MarkFlagRequired("title")

	ticketMoveCmd.Flags().StringVar(&ticketMoveColumn, "column", "", "target column id (required)")
	ticketMoveCmd.Flags().IntVar(&ticketMovePosition, "position", 0, "target position")
	_ = ticketMoveCmd.MarkFlagRequired("column")

	commentAddCmd.Flags().StringVar(&commentAddBody, "body", "", "comment body (required)")
	_ = commentAddCmd.MarkFlagRequired("body")

	ticketCmd.AddCommand(ticketListCmd, ticketGetCmd, ticketCreateCmd, ticketMoveCmd)
	commentCmd.AddCommand(commentListCmd, commentAddCmd)
}
