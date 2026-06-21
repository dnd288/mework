package postgres

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mework/server/bus"
)

// messageRow represents a row from the messages table.
type messageRow struct {
	ID      int64
	Topic   string
	Payload []byte
}

// insertMessage inserts a new message and returns its auto-generated ID.
func insertMessage(ctx context.Context, pool *pgxpool.Pool, topic string, payload []byte) (int64, error) {
	var id int64
	err := pool.QueryRow(ctx,
		`INSERT INTO messages (topic, payload) VALUES ($1, $2) RETURNING id`,
		topic, payload,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert message: %w", err)
	}
	return id, nil
}

// fetchUndelivered queries messages matching the given topic filter that have
// not been acknowledged. If fromID is >= 0, only messages with id > fromID are
// returned. Ordering is by id ascending for deterministic replay.
func fetchUndelivered(ctx context.Context, pool *pgxpool.Pool, topicPattern bus.Filter, fromID int64) ([]messageRow, error) {
	// We query by exact topic match for simplicity; the in-memory layer
	// applies the wildcard filter before returning to the subscriber.
	// For Postgres, we fetch all unacked messages matching the topic.
	rows, err := pool.Query(ctx,
		`SELECT id, topic, payload FROM messages
		 WHERE ($1 = '' OR topic = $1)
		   AND acked_at IS NULL
		   AND ($2 = 0 OR id > $2)
		 ORDER BY id`,
		string(topicPattern), fromID,
	)
	if err != nil {
		return nil, fmt.Errorf("fetch undelivered: %w", err)
	}
	defer rows.Close()

	var result []messageRow
	for rows.Next() {
		var row messageRow
		if err := rows.Scan(&row.ID, &row.Topic, &row.Payload); err != nil {
			return nil, fmt.Errorf("scan message row: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// fetchAllUndelivered returns all undelivered messages. Used for initial
// subscription replay when the subscriber uses a wildcard filter.
func fetchAllUndelivered(ctx context.Context, pool *pgxpool.Pool) ([]messageRow, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, topic, payload FROM messages
		 WHERE acked_at IS NULL
		 ORDER BY id`,
	)
	if err != nil {
		return nil, fmt.Errorf("fetch all undelivered: %w", err)
	}
	defer rows.Close()

	var result []messageRow
	for rows.Next() {
		var row messageRow
		if err := rows.Scan(&row.ID, &row.Topic, &row.Payload); err != nil {
			return nil, fmt.Errorf("scan message row: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// ackMessage marks a message as acknowledged, preventing future redelivery.
func ackMessage(ctx context.Context, pool *pgxpool.Pool, msgID int64) error {
	tag, err := pool.Exec(ctx,
		`UPDATE messages SET acked_at = NOW() WHERE id = $1 AND acked_at IS NULL`,
		msgID,
	)
	if err != nil {
		return fmt.Errorf("ack message: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// parseFromID converts the string fromEventID parameter to int64.
// Returns 0 when fromEventID is empty (meaning "from the beginning").
func parseFromID(fromEventID string) (int64, error) {
	if fromEventID == "" {
		return 0, nil
	}
	id, err := strconv.ParseInt(fromEventID, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse fromEventID %q: %w", fromEventID, err)
	}
	return id, nil
}
