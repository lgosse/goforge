package forgemongo_test

import (
	"testing"

	"github.com/lgosse/goforge/forgemongo"
)

type apiEntity struct {
	ID string `bson:"_id"`
}

type apiFilter struct {
	ID string `bson:"_id"`
}

type apiSort struct {
	ID int `bson:"_id"`
}

type apiPatch struct {
	Set any `bson:"$set"`
}

func TestPublicOptionsAPICompiles(t *testing.T) {
	t.Parallel()

	opts := []forgemongo.Option{
		forgemongo.WithFilter(apiFilter{ID: "entity-id"}),
		forgemongo.WithSort(apiSort{ID: 1}),
		forgemongo.WithLimit(10),
		forgemongo.WithSkip(20),
		forgemongo.WithAllDocuments(),
	}

	store := forgemongo.NewStore[apiEntity, apiFilter, apiSort, apiPatch](nil, "entities")

	if store == nil {
		t.Fatal("NewStore returned a nil Store")
	}
	if len(opts) != 5 {
		t.Fatalf("got %d options, want 5", len(opts))
	}
}
