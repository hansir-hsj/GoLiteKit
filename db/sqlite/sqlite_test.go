package sqlite_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hansir-hsj/GoLiteKit/db"
	_ "github.com/hansir-hsj/GoLiteKit/db/sqlite"
)

func TestNewFromConfigSQLite(t *testing.T) {
	dir := t.TempDir()
	dbFile := filepath.ToSlash(filepath.Join(dir, "app.db"))
	conf := filepath.Join(dir, "db.toml")
	content := fmt.Sprintf("[db]\ndriver = \"sqlite\"\ndsn = \"file:%s?_pragma=foreign_keys(1)\"\n", dbFile)
	if err := os.WriteFile(conf, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	gdb, err := db.NewFromConfig(conf)
	if err != nil {
		t.Fatalf("NewFromConfig: %v", err)
	}
	defer db.Close(gdb)

	type item struct {
		ID   int64
		Name string
	}
	if err := gdb.AutoMigrate(&item{}); err != nil {
		t.Fatal(err)
	}
	if err := gdb.Create(&item{Name: "a"}).Error; err != nil {
		t.Fatal(err)
	}
	var got item
	if err := gdb.First(&got).Error; err != nil || got.Name != "a" {
		t.Fatalf("got %+v, err %v", got, err)
	}
	var fk int
	if err := gdb.Raw("PRAGMA foreign_keys").Scan(&fk).Error; err != nil || fk != 1 {
		t.Fatalf("foreign_keys = %d, err %v; want pragma from dsn applied", fk, err)
	}
}
