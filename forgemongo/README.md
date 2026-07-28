# forgemongo

[![Go Reference](https://pkg.go.dev/badge/github.com/lgosse/goforge/forgemongo.svg)](https://pkg.go.dev/github.com/lgosse/goforge/forgemongo)

Package `forgemongo` provides a typed generic store on top of MongoDB Go Driver
v2.

Applications define their own entity, filter, sort, and native MongoDB update
types. The store supplies common CRUD operations, consistent GoForge errors,
explicit protection against unfiltered writes, streaming, and a configurable
test mock.

## Install

```sh
go get github.com/lgosse/goforge/forgemongo@latest
```

## Defining a store

```go
type User struct {
	ID     string   `bson:"_id"`
	Status string   `bson:"status"`
	Tags   []string `bson:"tags"`
}

type UserFilter struct {
	ID     string `bson:"_id,omitempty"`
	Status string `bson:"status,omitempty"`
}

type UserSort struct {
	CreatedAt int `bson:"created_at"`
}

type UserPatch struct {
	Set *struct {
		Status string `bson:"status,omitempty"`
	} `bson:"$set,omitempty"`
	Push *struct {
		Tags string `bson:"tags,omitempty"`
	} `bson:"$push,omitempty"`
}

users := forgemongo.NewStore[User, UserFilter, UserSort, UserPatch](
	database,
	"users",
)
```

Update values are passed directly to the MongoDB driver. Define `$set`,
`$push`, `$inc`, and other operators explicitly in the patch type.

## Instrumenting the MongoDB client

`forgemongo` does not initialize or own MongoDB clients. Applications that use
the [`otel/mongo`](https://pkg.go.dev/github.com/lgosse/goforge/otel/mongo)
module attach telemetry while constructing their driver options:

```go
monitor, err := forgeotelmongo.NewMonitor(telemetry)
if err != nil {
	return err
}

clientOptions := mongooptions.Client().
	ApplyURI(mongoURI).
	SetMonitor(monitor)

client, err := mongo.Connect(clientOptions)
if err != nil {
	return err
}

users := forgemongo.NewStore[User, UserFilter, UserSort, UserPatch](
	client.Database("users"),
	"users",
)
```

The application remains responsible for connecting, checking, and
disconnecting the client.

## Querying

```go
activeUsers, err := users.FindMany(
	ctx,
	forgemongo.WithFilter(UserFilter{Status: "active"}),
	forgemongo.WithSort(UserSort{CreatedAt: -1}),
	forgemongo.WithLimit(100),
	forgemongo.WithSkip(200),
)
if err != nil {
	return err
}

fmt.Println(len(activeUsers))
```

Multiple filters are combined with `$or`. Filter and sort values are checked
against the store's generic types before the database is called.

Use `Stream` when results should be decoded incrementally:

```go
for user, err := range users.Stream(
	ctx,
	forgemongo.WithFilter(UserFilter{Status: "active"}),
) {
	if err != nil {
		return err
	}

	process(user)
}
```

## Safe writes

Patch and delete operations require a filter:

```go
err := users.DeleteOne(
	ctx,
	forgemongo.WithFilter(UserFilter{ID: "user-1"}),
)
```

Bulk operations across the complete collection must be explicit:

```go
count, err := users.DeleteMany(
	ctx,
	forgemongo.WithAllDocuments(),
)
```

Read-only options such as limit and skip are rejected on writes. Sort is only
accepted by `PatchOne`, where MongoDB can use it to select the updated
document.

## Testing with a mock

```go
store := forgemongo.NewMock[User, UserFilter, UserSort, UserPatch]()
store.FindOneFunc = func(
	ctx context.Context,
	opts ...forgemongo.Option,
) (*User, error) {
	return &User{ID: "user-1", Status: "active"}, nil
}

user, err := store.FindOne(
	context.Background(),
	forgemongo.WithFilter(UserFilter{ID: "user-1"}),
)
if err != nil {
	log.Fatal(err)
}

fmt.Println(user.ID)
fmt.Println(store.Calls().FindOne) // 1
```

All mock methods are configurable and maintain concurrency-safe invocation
counts. An unconfigured method returns a descriptive error rather than
panicking.

## License

This module is available under the repository's
[MIT License](../LICENCE.txt).
