package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"game-realtime-gm/backend/internal/config"
	"game-realtime-gm/backend/internal/database"
)

func main() {
	defaultPath := "internal/database/migrations/day37_v2_pve_foundation.sql"
	path := flag.String("migration", defaultPath, "V2 migration file")
	stage := flag.String("stage", "r1", "r1, r2, r3 or r4 migration chain")
	flag.Parse()
	if *stage != "r1" && *stage != "r2" && *stage != "r3" && *stage != "r4" {
		log.Fatal("stage must be r1, r2, r3 or r4")
	}
	if os.Getenv("V2_MIGRATION_CONFIRM") != "I_UNDERSTAND_V2_MIGRATION" {
		log.Fatal("set V2_MIGRATION_CONFIRM=I_UNDERSTAND_V2_MIGRATION before applying or reconciling the V2 migration")
	}
	cfg := config.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	db, err := database.NewMySQLDB(ctx, cfg.Database)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	status, err := database.ApplyV2Foundation(ctx, db, *path)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(status)
	if *stage == "r2" || *stage == "r3" || *stage == "r4" {
		status, err = database.ApplyV2Products(ctx, db, "internal/database/migrations/day38_v2_product_model.sql")
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(status)
		if *stage == "r3" || *stage == "r4" {
			status, err = database.ApplyV2Engineering(ctx, db, "internal/database/migrations/day39_v2_engineering.sql")
			if err != nil {
				log.Fatal(err)
			}
			fmt.Println(status)
			if *stage == "r4" {
				status, err = database.ApplyV2Retirement(ctx, db, "internal/database/migrations/day40_v2_retirement.sql")
				if err != nil {
					log.Fatal(err)
				}
				fmt.Println(status)
			}
		}
	} else if *stage != "r1" {
		log.Fatal("stage must be r1 or r2")
	}
}
