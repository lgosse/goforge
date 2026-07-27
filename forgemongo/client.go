package forgemongo

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"net/http"

	"github.com/lgosse/goforge"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	mongooptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	// ErrorCodeInvalidArgument identifies invalid store input or query options.
	ErrorCodeInvalidArgument = "ERR_MONGO_INVALID_ARGUMENT"
	// ErrorCodeNotFound identifies a MongoDB operation that matched no document.
	ErrorCodeNotFound = "ERR_MONGO_NOT_FOUND"
	// ErrorCodeConflict identifies a duplicate-key write.
	ErrorCodeConflict = "ERR_MONGO_CONFLICT"
	// ErrorCodeTimeout identifies a MongoDB timeout or expired context deadline.
	ErrorCodeTimeout = "ERR_MONGO_TIMEOUT"
	// ErrorCodeCanceled identifies an operation canceled by its caller.
	ErrorCodeCanceled = "ERR_MONGO_CANCELED"
	// ErrorCodeDatabase identifies an unclassified MongoDB failure.
	ErrorCodeDatabase = "ERR_MONGO_DATABASE"
)

var (
	errNilDatabase        = errors.New("forgemongo: nil database")
	errNilCollection      = errors.New("forgemongo: nil collection")
	errNilEntity          = errors.New("forgemongo: nil entity")
	errEmptyEntities      = errors.New("forgemongo: no entities to insert")
	errMissingWriteFilter = errors.New("forgemongo: write operation requires WithFilter or WithAllDocuments")
)

type collection interface {
	FindOne(
		ctx context.Context,
		filter any,
		opts ...mongooptions.Lister[mongooptions.FindOneOptions],
	) *mongo.SingleResult
	Find(
		ctx context.Context,
		filter any,
		opts ...mongooptions.Lister[mongooptions.FindOptions],
	) (*mongo.Cursor, error)
	InsertOne(
		ctx context.Context,
		document any,
		opts ...mongooptions.Lister[mongooptions.InsertOneOptions],
	) (*mongo.InsertOneResult, error)
	InsertMany(
		ctx context.Context,
		documents any,
		opts ...mongooptions.Lister[mongooptions.InsertManyOptions],
	) (*mongo.InsertManyResult, error)
	UpdateOne(
		ctx context.Context,
		filter any,
		update any,
		opts ...mongooptions.Lister[mongooptions.UpdateOneOptions],
	) (*mongo.UpdateResult, error)
	UpdateMany(
		ctx context.Context,
		filter any,
		update any,
		opts ...mongooptions.Lister[mongooptions.UpdateManyOptions],
	) (*mongo.UpdateResult, error)
	DeleteOne(
		ctx context.Context,
		filter any,
		opts ...mongooptions.Lister[mongooptions.DeleteOneOptions],
	) (*mongo.DeleteResult, error)
	DeleteMany(
		ctx context.Context,
		filter any,
		opts ...mongooptions.Lister[mongooptions.DeleteManyOptions],
	) (*mongo.DeleteResult, error)
}

var _ collection = (*mongo.Collection)(nil)

type mongoStore[T, F, S, P any] struct {
	coll    collection
	initErr error
}

// NewStore creates a typed store for collName in db.
//
// A nil database is retained as a configuration error and returned by store
// operations instead of causing a panic during construction.
func NewStore[T, F, S, P any](db *mongo.Database, collName string) Store[T, F, S, P] {
	store := &mongoStore[T, F, S, P]{}
	if db == nil {
		store.initErr = errNilDatabase
		return store
	}

	store.coll = db.Collection(collName)
	return store
}

func (s *mongoStore[T, F, S, P]) FindOne(ctx context.Context, opts ...Option) (*T, error) {
	query, err := compileQuery[F, S](opts)
	if err != nil {
		return nil, newInvalidArgumentError(err)
	}
	if err := s.ready(); err != nil {
		return nil, newDatabaseError(err)
	}

	result := s.coll.FindOne(ctx, query.filter, query.findOneOptions())
	if result == nil {
		return nil, newDatabaseError(errors.New("find one: mongodb returned a nil result"))
	}

	var entity T
	if err := result.Decode(&entity); err != nil {
		return nil, handleMongoError(fmt.Errorf("find one: %w", err))
	}

	return &entity, nil
}

func (s *mongoStore[T, F, S, P]) FindMany(ctx context.Context, opts ...Option) ([]*T, error) {
	query, err := compileQuery[F, S](opts)
	if err != nil {
		return nil, newInvalidArgumentError(err)
	}
	if err := s.ready(); err != nil {
		return nil, newDatabaseError(err)
	}

	cursor, err := s.coll.Find(ctx, query.filter, query.findOptions())
	if err != nil {
		return nil, handleMongoError(fmt.Errorf("find many: %w", err))
	}
	if cursor == nil {
		return nil, newDatabaseError(errors.New("find many: mongodb returned a nil cursor"))
	}

	var values []T
	if err := cursor.All(ctx, &values); err != nil {
		return nil, handleMongoError(fmt.Errorf("find many: %w", err))
	}

	entities := make([]*T, len(values))
	for idx := range values {
		entities[idx] = &values[idx]
	}

	return entities, nil
}

func (s *mongoStore[T, F, S, P]) Stream(ctx context.Context, opts ...Option) iter.Seq2[*T, error] {
	query, queryErr := compileQuery[F, S](opts)

	return func(yield func(*T, error) bool) {
		if queryErr != nil {
			yield(nil, newInvalidArgumentError(queryErr))
			return
		}
		if err := s.ready(); err != nil {
			yield(nil, newDatabaseError(err))
			return
		}

		cursor, err := s.coll.Find(ctx, query.filter, query.findOptions())
		if err != nil {
			yield(nil, handleMongoError(fmt.Errorf("stream: %w", err)))
			return
		}
		if cursor == nil {
			yield(nil, newDatabaseError(errors.New("stream: mongodb returned a nil cursor")))
			return
		}
		defer func() {
			_ = cursor.Close(context.Background())
		}()

		for cursor.Next(ctx) {
			var entity T
			if err := cursor.Decode(&entity); err != nil {
				yield(nil, handleMongoError(fmt.Errorf("stream decode: %w", err)))
				return
			}
			if !yield(&entity, nil) {
				return
			}
		}

		if err := cursor.Err(); err != nil {
			yield(nil, handleMongoError(fmt.Errorf("stream: %w", err)))
		}
	}
}

func (s *mongoStore[T, F, S, P]) InsertOne(ctx context.Context, entity *T) error {
	if entity == nil {
		return newInvalidArgumentError(errNilEntity)
	}
	if err := s.ready(); err != nil {
		return newDatabaseError(err)
	}

	if _, err := s.coll.InsertOne(ctx, entity); err != nil {
		return handleMongoError(fmt.Errorf("insert one: %w", err))
	}

	return nil
}

func (s *mongoStore[T, F, S, P]) InsertMany(ctx context.Context, entities []*T) error {
	if len(entities) == 0 {
		return newInvalidArgumentError(errEmptyEntities)
	}
	for _, entity := range entities {
		if entity == nil {
			return newInvalidArgumentError(errNilEntity)
		}
	}
	if err := s.ready(); err != nil {
		return newDatabaseError(err)
	}

	if _, err := s.coll.InsertMany(ctx, entities); err != nil {
		return handleMongoError(fmt.Errorf("insert many: %w", err))
	}

	return nil
}

func (s *mongoStore[T, F, S, P]) PatchOne(ctx context.Context, update P, opts ...Option) error {
	query, err := compileQuery[F, S](opts)
	if err != nil {
		return newInvalidArgumentError(err)
	}
	if err := query.validateWrite(true); err != nil {
		return newInvalidArgumentError(err)
	}
	if err := s.ready(); err != nil {
		return newDatabaseError(err)
	}

	result, err := s.coll.UpdateOne(ctx, query.filter, update, query.updateOneOptions())
	if err != nil {
		return handleMongoError(fmt.Errorf("patch one: %w", err))
	}
	if result == nil {
		return newDatabaseError(errors.New("patch one: mongodb returned a nil result"))
	}
	if result.MatchedCount == 0 {
		return newNotFoundError(fmt.Errorf("patch one: %w", mongo.ErrNoDocuments), "document not found to patch")
	}

	return nil
}

func (s *mongoStore[T, F, S, P]) PatchMany(ctx context.Context, update P, opts ...Option) (int64, error) {
	query, err := compileQuery[F, S](opts)
	if err != nil {
		return 0, newInvalidArgumentError(err)
	}
	if err := query.validateWrite(false); err != nil {
		return 0, newInvalidArgumentError(err)
	}
	if err := s.ready(); err != nil {
		return 0, newDatabaseError(err)
	}

	result, err := s.coll.UpdateMany(ctx, query.filter, update)
	if err != nil {
		return 0, handleMongoError(fmt.Errorf("patch many: %w", err))
	}
	if result == nil {
		return 0, newDatabaseError(errors.New("patch many: mongodb returned a nil result"))
	}

	return result.ModifiedCount, nil
}

func (s *mongoStore[T, F, S, P]) DeleteOne(ctx context.Context, opts ...Option) error {
	query, err := compileQuery[F, S](opts)
	if err != nil {
		return newInvalidArgumentError(err)
	}
	if err := query.validateWrite(false); err != nil {
		return newInvalidArgumentError(err)
	}
	if err := s.ready(); err != nil {
		return newDatabaseError(err)
	}

	result, err := s.coll.DeleteOne(ctx, query.filter)
	if err != nil {
		return handleMongoError(fmt.Errorf("delete one: %w", err))
	}
	if result == nil {
		return newDatabaseError(errors.New("delete one: mongodb returned a nil result"))
	}
	if result.DeletedCount == 0 {
		return newNotFoundError(fmt.Errorf("delete one: %w", mongo.ErrNoDocuments), "document not found to delete")
	}

	return nil
}

func (s *mongoStore[T, F, S, P]) DeleteMany(ctx context.Context, opts ...Option) (int64, error) {
	query, err := compileQuery[F, S](opts)
	if err != nil {
		return 0, newInvalidArgumentError(err)
	}
	if err := query.validateWrite(false); err != nil {
		return 0, newInvalidArgumentError(err)
	}
	if err := s.ready(); err != nil {
		return 0, newDatabaseError(err)
	}

	result, err := s.coll.DeleteMany(ctx, query.filter)
	if err != nil {
		return 0, handleMongoError(fmt.Errorf("delete many: %w", err))
	}
	if result == nil {
		return 0, newDatabaseError(errors.New("delete many: mongodb returned a nil result"))
	}

	return result.DeletedCount, nil
}

func (s *mongoStore[T, F, S, P]) ready() error {
	if s == nil {
		return errNilCollection
	}
	if s.initErr != nil {
		return s.initErr
	}
	if s.coll == nil {
		return errNilCollection
	}

	return nil
}

type compiledQuery struct {
	filter            any
	sort              any
	limit             *int64
	skip              *int64
	hasFilters        bool
	allowAllDocuments bool
}

func compileQuery[F, S any](opts []Option) (compiledQuery, error) {
	var config QueryConfig
	for _, opt := range opts {
		if opt != nil {
			opt(&config)
		}
	}

	if config.Limit != nil && *config.Limit < 0 {
		return compiledQuery{}, fmt.Errorf("limit must be non-negative, got %d", *config.Limit)
	}
	if config.Skip != nil && *config.Skip < 0 {
		return compiledQuery{}, fmt.Errorf("skip must be non-negative, got %d", *config.Skip)
	}

	filters := make([]any, len(config.Filters))
	for idx, untypedFilter := range config.Filters {
		filter, ok := untypedFilter.(F)
		if !ok {
			return compiledQuery{}, fmt.Errorf(
				"filter %d has type %T, expected the store filter type",
				idx,
				untypedFilter,
			)
		}
		filters[idx] = filter
	}

	var sort any
	if config.Sort != nil {
		typedSort, ok := config.Sort.(S)
		if !ok {
			return compiledQuery{}, fmt.Errorf(
				"sort has type %T, expected the store sort type",
				config.Sort,
			)
		}
		sort = typedSort
	}

	filter := any(bson.D{})
	switch len(filters) {
	case 1:
		filter = filters[0]
	default:
		if len(filters) > 1 {
			filter = bson.D{{Key: "$or", Value: filters}}
		}
	}

	var limit *int64
	if config.Limit != nil {
		value := *config.Limit
		limit = &value
	}
	var skip *int64
	if config.Skip != nil {
		value := *config.Skip
		skip = &value
	}

	return compiledQuery{
		filter:            filter,
		sort:              sort,
		limit:             limit,
		skip:              skip,
		hasFilters:        len(filters) > 0,
		allowAllDocuments: config.AllowAllDocuments,
	}, nil
}

func (q compiledQuery) findOptions() *mongooptions.FindOptionsBuilder {
	findOptions := mongooptions.Find()
	if q.sort != nil {
		findOptions.SetSort(q.sort)
	}
	if q.limit != nil {
		findOptions.SetLimit(*q.limit)
	}
	if q.skip != nil {
		findOptions.SetSkip(*q.skip)
	}

	return findOptions
}

func (q compiledQuery) findOneOptions() *mongooptions.FindOneOptionsBuilder {
	findOneOptions := mongooptions.FindOne()
	if q.sort != nil {
		findOneOptions.SetSort(q.sort)
	}
	if q.skip != nil {
		findOneOptions.SetSkip(*q.skip)
	}

	return findOneOptions
}

func (q compiledQuery) updateOneOptions() *mongooptions.UpdateOneOptionsBuilder {
	updateOptions := mongooptions.UpdateOne()
	if q.sort != nil {
		updateOptions.SetSort(q.sort)
	}

	return updateOptions
}

func (q compiledQuery) validateWrite(allowSort bool) error {
	if q.limit != nil {
		return errors.New("forgemongo: WithLimit is not valid for write operations")
	}
	if q.skip != nil {
		return errors.New("forgemongo: WithSkip is not valid for write operations")
	}
	if q.sort != nil && !allowSort {
		return errors.New("forgemongo: WithSort is only valid for PatchOne writes")
	}
	if !q.hasFilters && !q.allowAllDocuments {
		return errMissingWriteFilter
	}

	return nil
}

func handleMongoError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, mongo.ErrNoDocuments):
		return newNotFoundError(err, "document not found")
	case mongo.IsDuplicateKeyError(err):
		return goforge.NewError(err).
			WithHTTPStatus(http.StatusConflict).
			WithCode(ErrorCodeConflict).
			WithMessage("document conflicts with an existing record")
	case errors.Is(err, context.DeadlineExceeded), mongo.IsTimeout(err):
		return goforge.NewError(err).
			WithHTTPStatus(http.StatusGatewayTimeout).
			WithCode(ErrorCodeTimeout).
			WithMessage("database operation timed out")
	case errors.Is(err, context.Canceled):
		return goforge.NewError(err).
			WithHTTPStatus(http.StatusRequestTimeout).
			WithCode(ErrorCodeCanceled).
			WithMessage("database operation canceled")
	default:
		return newDatabaseError(err)
	}
}

func newInvalidArgumentError(err error) error {
	return goforge.NewError(err).
		WithHTTPStatus(http.StatusBadRequest).
		WithCode(ErrorCodeInvalidArgument).
		WithMessage("invalid MongoDB operation")
}

func newNotFoundError(err error, message string) error {
	return goforge.NewError(err).
		WithHTTPStatus(http.StatusNotFound).
		WithCode(ErrorCodeNotFound).
		WithMessage(message)
}

func newDatabaseError(err error) error {
	return goforge.NewError(err).
		WithHTTPStatus(http.StatusInternalServerError).
		WithCode(ErrorCodeDatabase).
		WithMessage("database operation failed")
}
