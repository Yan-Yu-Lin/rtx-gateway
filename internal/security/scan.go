package security

import (
	"database/sql"
	"time"
)

func scanBan(scanner interface{ Scan(dest ...any) error }) (Ban, error) {
	var ban Ban
	var bannedUntil string
	var manual int
	var createdAt string
	var updatedAt string
	var liftedAt sql.NullString
	err := scanner.Scan(
		&ban.ID,
		&ban.ClientIP,
		&ban.Reason,
		&ban.Strikes,
		&bannedUntil,
		&manual,
		&createdAt,
		&updatedAt,
		&liftedAt,
	)
	if err != nil {
		return Ban{}, err
	}
	ban.Manual = manual == 1
	ban.BannedUntil, err = time.Parse(time.RFC3339Nano, bannedUntil)
	if err != nil {
		return Ban{}, err
	}
	ban.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return Ban{}, err
	}
	ban.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return Ban{}, err
	}
	if liftedAt.Valid {
		parsed, err := time.Parse(time.RFC3339Nano, liftedAt.String)
		if err != nil {
			return Ban{}, err
		}
		ban.LiftedAt = &parsed
	}
	return ban, nil
}

func scanEvent(scanner interface{ Scan(dest ...any) error }) (Event, error) {
	var event Event
	var host sql.NullString
	var path sql.NullString
	var statusCode sql.NullInt64
	var detail sql.NullString
	var createdAt string
	err := scanner.Scan(
		&event.ID,
		&event.ClientIP,
		&event.EventType,
		&host,
		&path,
		&statusCode,
		&detail,
		&createdAt,
	)
	if err != nil {
		return Event{}, err
	}
	if host.Valid {
		event.Host = host.String
	}
	if path.Valid {
		event.Path = path.String
	}
	if statusCode.Valid {
		value := int(statusCode.Int64)
		event.StatusCode = &value
	}
	if detail.Valid {
		event.Detail = detail.String
	}
	parsed, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return Event{}, err
	}
	event.CreatedAt = parsed
	return event, nil
}
