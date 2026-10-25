package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Pool struct {
	*pgxpool.Pool
}

func Connect(ctx context.Context, url string) (*Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 20
	cfg.MinConns = 2
	cfg.MaxConnLifetime = 30 * time.Minute

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
	return err != nil && containsCode(err, "23505")
}

func IsForeignKeyViolation(err error) bool {
	return err != nil && containsCode(err, "23503")
}

func containsCode(err error, code string) bool {
	type coded interface{ SQLState() string }
	for err != nil {
		if c, ok := err.(coded); ok && c.SQLState() == code {
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			break
		}
		err = u.Unwrap()
	}
	return false
}
