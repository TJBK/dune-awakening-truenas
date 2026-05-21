package main

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var globalDB *pgxpool.Pool

func cmdConnect() tea.Msg {
	pool, err := connectDB(context.Background(), dbUser, dbPass)
	if err != nil {
		return msgConnect{err: fmt.Errorf("DB connect: %w", err)}
	}
	globalDB = pool
	return msgConnect{}
}

func connectDB(ctx context.Context, user, pass string) (*pgxpool.Pool, error) {
	parts := []string{
		"host=127.0.0.1",
		fmt.Sprintf("port=%d", dbPort),
		fmt.Sprintf("user=%s", user),
		fmt.Sprintf("dbname=%s", dbName),
		"sslmode=disable",
	}
	if pass != "" {
		parts = append(parts, fmt.Sprintf("password=%s", pass))
	}

	poolCfg, err := pgxpool.ParseConfig(strings.Join(parts, " "))
	if err != nil {
		return nil, err
	}
	poolCfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, fmt.Sprintf(`SET search_path TO %s, public`, pgx.Identifier{dbSchema}.Sanitize()))
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	dbUser = user
	dbPass = pass
	return pool, nil
}
