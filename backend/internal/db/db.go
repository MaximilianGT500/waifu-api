package db

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Pool struct {
	*pgxpool.Pool
}

type Options struct {
	MaxConns         int
	MinConns         int
	StatementTimeout time.Duration
	ApplicationName  string
	KeepAlive        bool
}

func Connect(ctx context.Context, url string, opts Options) (*Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	maxConns := opts.MaxConns
	if maxConns <= 0 {
		maxConns = 20
	}
	cfg.MaxConns = int32(maxConns)
	cfg.MinConns = int32(min(max(opts.MinConns, 0), maxConns))
	if opts.MinConns == 0 {
		cfg.MinConns = min(2, int32(maxConns))
	}
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.HealthCheckPeriod = 30 * time.Second
	cfg.ConnConfig.ConnectTimeout = 10 * time.Second
	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	if opts.StatementTimeout > 0 {
		cfg.ConnConfig.RuntimeParams["statement_timeout"] = strconv.FormatInt(opts.StatementTimeout.Milliseconds(), 10)
	}
	cfg.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = "60000"
	name := opts.ApplicationName
	if name == "" {
		name = "waifu-api"
	}
	if len(name) > 63 {
		name = name[:63]
	}
	cfg.ConnConfig.RuntimeParams["application_name"] = name
	if opts.KeepAlive {
		cfg.ConnConfig.RuntimeParams["tcp_keepalives_idle"] = "15"
		cfg.ConnConfig.RuntimeParams["tcp_keepalives_interval"] = "5"
		cfg.ConnConfig.RuntimeParams["tcp_keepalives_count"] = "3"
	}

	p, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := p.Ping(pingCtx); err != nil {
		p.Close()
		return nil, err
	}
	return &Pool{Pool: p}, nil
}

func IsUniqueViolation(err error) bool {
	return hasCode(err, "23505")
}

func IsForeignKeyViolation(err error) bool {
	return hasCode(err, "23503")
}

func ConstraintName(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.ConstraintName
	}
	return ""
}

func hasCode(err error, code string) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == code
	}
	type coded interface{ SQLState() string }
	var c coded
	if errors.As(err, &c) {
		return c.SQLState() == code
	}
	return false
}
