package database

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Database struct {
	pool *pgxpool.Pool
}

func NewDatabase(ctx context.Context, pool *pgxpool.Pool) *Database {
	initialise(ctx, pool)
	return &Database{
		pool: pool,
	}
}

func initialise(ctx context.Context, pool *pgxpool.Pool) {
	const createPropertiesTableSQL = `
		CREATE TABLE IF NOT EXISTS Property (
				id 					SERIAL PRIMARY KEY,
				time_stamp 			TIMESTAMP WITH TIME ZONE NOT NULL,
				unit_of_measurement TEXT NOT NULL,
				value				TEXT NOT NULL,
				identifier 			TEXT NOT NULL,
				slug 				TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_properties_identifier ON Property (identifier);
		CREATE INDEX IF NOT EXISTS idx_properties_timestamp ON Property (time_stamp);
		`
	if _, err := pool.Exec(ctx, createPropertiesTableSQL); err != nil {
		panic(err)
	}

	const createDeviceTableSQL = `
	CREATE TABLE IF NOT EXISTS Device (
		id           	TEXT PRIMARY KEY,
		model        	TEXT,
		serial_number 	TEXT,
		created_at 		TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);
	`
	if _, err := pool.Exec(ctx, createDeviceTableSQL); err != nil {
		panic(err)
	}

	const createAmberPriceTableSQL = `
	CREATE TABLE IF NOT EXISTS AmberPrice (
		id 				SERIAL PRIMARY KEY,
		per_kwh        	NUMERIC(10, 5) NOT NULL,
		spot_per_kwh    NUMERIC(10, 5) NOT NULL,
		start_time 		TIMESTAMP WITH TIME ZONE NOT NULL,
		end_time 		TIMESTAMP WITH TIME ZONE NOT NULL,
		duration 		INT NOT NULL,
		forecast 		BOOL NOT NULL DEFAULT FALSE,
		channel_type 	TEXT NOT NULL,
		created_at 		TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at 		TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_amber_price_start_time ON AmberPrice (start_time);
	CREATE INDEX IF NOT EXISTS idx_amber_price_end_time ON AmberPrice (end_time);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_amber_price_unique_start_time_channel_type ON AmberPrice (start_time, channel_type);
	`
	if _, err := pool.Exec(ctx, createAmberPriceTableSQL); err != nil {
		panic(err)
	}
}
