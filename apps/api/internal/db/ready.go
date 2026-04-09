package db

import "context"

func CheckReadiness(ctx context.Context, cfg Config) error {
	pool, err := NewPool(ctx, cfg)
	if err != nil {
		return err
	}
	defer pool.Close()
	return nil
}
