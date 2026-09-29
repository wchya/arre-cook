// nutritioncatalog validates and explicitly imports reviewed food data.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"io"
	"ninimenu/internal/models"
	"ninimenu/internal/services"
	"os"
)

func decodeCatalog(reader io.Reader) ([]services.CatalogFoodInput, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, 16*1024*1024+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > 16*1024*1024 {
		return nil, errors.New("input exceeds 16 MiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var inputs []services.CatalogFoodInput
	if err := decoder.Decode(&inputs); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, errors.New("input must contain exactly one JSON array")
	}
	if _, err := services.ValidateCatalogFoods(inputs); err != nil {
		return nil, err
	}
	return inputs, nil
}

func run(args []string, output io.Writer) error {
	flags := flag.NewFlagSet("nutritioncatalog", flag.ContinueOnError)
	flags.SetOutput(output)
	file := flags.String("file", "", "reviewed JSON catalog input; default mode only validates")
	apply := flags.Bool("apply", false, "write the validated batch in one transaction")
	sqlitePath := flags.String("sqlite", "", "target SQLite path")
	useMySQL := flags.Bool("mysql", false, "use MYSQL_DSN from environment")
	migrate := flags.Bool("migrate", false, "create/upgrade catalog table (only with -apply)")
	withdraw := flags.Uint("withdraw", 0, "disable one catalog ID; keeps personal and historical snapshots")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if *migrate && !*apply {
		return errors.New("-migrate requires -apply")
	}
	var inputs []services.CatalogFoodInput
	if *withdraw > 0 {
		if *file != "" || !*apply {
			return errors.New("withdraw requires -apply and cannot be combined with -file")
		}
	} else {
		if *file == "" {
			return errors.New("-file is required")
		}
		f, err := os.Open(*file)
		if err != nil {
			return err
		}
		defer f.Close()
		inputs, err = decodeCatalog(f)
		if err != nil {
			return err
		}
	}
	if !*apply {
		fmt.Fprintf(output, "Validated %d items; no database opened or modified.\n", len(inputs))
		return nil
	}
	if (*sqlitePath == "") == (!*useMySQL) {
		return errors.New("choose exactly one of -sqlite or -mysql")
	}
	var dialector gorm.Dialector
	if *useMySQL {
		dsn := os.Getenv("MYSQL_DSN")
		if dsn == "" {
			return errors.New("MYSQL_DSN is required")
		}
		dialector = mysql.Open(dsn)
	} else {
		dialector = sqlite.Open(*sqlitePath)
	}
	db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return errors.New("cannot open catalog database; check configured target")
	}
	pool, err := db.DB()
	if err != nil {
		return err
	}
	defer pool.Close()
	if *migrate {
		if err := db.AutoMigrate(&models.NutritionCatalogFood{}); err != nil {
			return err
		}
	}
	if *withdraw > 0 {
		if err := services.WithdrawNutritionCatalog(*withdraw, db); err != nil {
			return err
		}
		fmt.Fprintf(output, "Catalog withdrawal completed for ID %d.\n", *withdraw)
		return nil
	}
	if err := services.ImportNutritionCatalog(inputs, db); err != nil {
		return err
	}
	fmt.Fprintf(output, "Imported or verified %d immutable catalog items.\n", len(inputs))
	return nil
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
