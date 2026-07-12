package main

import (
	"os"

	"github.com/Its-Ameekh/school_software_backend/internal/app"
	"github.com/Its-Ameekh/school_software_backend/internal/config"
	"github.com/Its-Ameekh/school_software_backend/internal/database"
	"github.com/Its-Ameekh/school_software_backend/internal/logger"

	_ "github.com/Its-Ameekh/school_software_backend/docs" // swag-generated docs package; run `swag init` first
)

// @title           School Software API
// @version         1.0
// @description     Backend API for the School Software preschool management system.
// @BasePath        /

func main() {
	// 1. Config first — nothing else can safely start if required
	//    settings are missing. config.Load() fatals internally on its
	//    own if something required is blank.
	cfg := config.Load()

	// 2. Logger next — every step after this should log through it,
	//    not fmt.Println, so boot failures show up in the same
	//    structured format as everything else.
	log := logger.New(cfg.Environment)

	// 3. Database — the one dependency almost everything else needs.
	//    A failed connection here is fatal; there's nothing useful the
	//    app can do without it.
	db, err := database.Connect(cfg.DatabaseURL, log)
	if err != nil {
		log.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}

	// 4. Container — bundles config/logger/db so nothing downstream
	//    reaches for package-level globals.
	container := app.New(cfg, log, db)

	// 5. Router — depends on the container to wire middleware and
	//    routes (currently just /health; Stage 4 adds the rest).
	router := app.NewRouter(container)

	// 6. Server — blocks here until SIGINT/SIGTERM, then drains
	//    in-flight requests and exits cleanly.
	if err := app.RunServer(container, router); err != nil {
		log.Error("server exited with error", "error", err)
		os.Exit(1)
	}
}
