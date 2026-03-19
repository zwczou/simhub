package utils

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

func TestFormOrder(t *testing.T) {
	db := bun.NewDB(&sql.DB{}, pgdialect.New())

	type MyReq struct {
		Sort  string
		Order string
	}

	req := MyReq{Sort: "created_at", Order: "asc"}

	q := db.NewSelect()
	q = NewQueryForm(&req).Order(q)

	qs := q.String()
	if !strings.Contains(qs, `"created_at" ASC NULLS LAST`) {
		t.Errorf("unexpected query: %s", qs)
	}

	req2 := MyReq{Sort: "id", Order: "desc"}
	q2 := db.NewSelect()
	q2 = NewQueryForm(&req2).NullsFirst().SetAlias("user").Order(q2)
	qs2 := q2.String()
	if !strings.Contains(qs2, `"user"."id" DESC`) {
		t.Errorf("unexpected query: %s", qs2)
	}
}
