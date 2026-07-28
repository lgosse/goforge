package forgemongo_test

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/lgosse/goforge/forgemongo"
)

type exampleUser struct {
	ID string `bson:"_id"`
}

type exampleUserFilter struct {
	ID string `bson:"_id"`
}

type exampleUserSort struct {
	ID int `bson:"_id"`
}

type exampleUserPatch struct {
	Set any `bson:"$set"`
}

func ExampleNewMock() {
	store := forgemongo.NewMock[
		exampleUser,
		exampleUserFilter,
		exampleUserSort,
		exampleUserPatch,
	]()
	store.FindOneFunc = func(_ context.Context, opts ...forgemongo.Option) (*exampleUser, error) {
		fmt.Println("query options:", len(opts))
		return &exampleUser{ID: "user-1"}, nil
	}

	user, err := store.FindOne(
		context.Background(),
		forgemongo.WithFilter(exampleUserFilter{ID: "user-1"}),
		forgemongo.WithProjection(bson.D{{Key: "_id", Value: 1}}),
	)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(user.ID)
	fmt.Println("find calls:", store.Calls().FindOne)
	// Output:
	// query options: 2
	// user-1
	// find calls: 1
}
