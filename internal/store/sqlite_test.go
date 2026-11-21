package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/520yxl/fnsdownload/internal/engine"
)

func TestTaskLifecycle(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "data", "tasks.db"))
	if err != nil { t.Fatal(err) }
	defer db.Close()
	ctx := context.Background()
	now := time.Now().UTC()
	task := engine.Task{ID:"task-1",Engine:"http",Name:"archive.zip",Status:engine.StatusQueued,CreatedAt:now,UpdatedAt:now}
	if err := db.Upsert(ctx, task); err != nil { t.Fatal(err) }
	items, err := db.List(ctx)
	if err != nil { t.Fatal(err) }
	if len(items)!=1 || items[0].ID!=task.ID || items[0].Name!=task.Name { t.Fatalf("unexpected tasks: %#v",items) }
	task.Status=engine.StatusPaused
	task.CompletedBytes=128
	task.UpdatedAt=time.Now().UTC()
	if err := db.Upsert(ctx,task);err!=nil{t.Fatal(err)}
	items,err=db.List(ctx);if err!=nil{t.Fatal(err)}
	if len(items)!=1||items[0].Status!=engine.StatusPaused||items[0].CompletedBytes!=128{t.Fatalf("upsert did not replace task: %#v",items)}
	if err:=db.Delete(ctx,task.ID);err!=nil{t.Fatal(err)}
	items,err=db.List(ctx);if err!=nil{t.Fatal(err)}
	if len(items)!=0{t.Fatalf("expected empty task list, got %#v",items)}
}
