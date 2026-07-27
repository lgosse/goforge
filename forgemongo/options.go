package forgemongo

// QueryConfig is the configuration assembled by query options.
//
// Filter and sort values are stored as any so options such as [WithLimit] do
// not require explicit generic type arguments. A Store validates their concrete
// types against its F and S parameters before calling MongoDB.
type QueryConfig struct {
	// Filters contains the MongoDB filters to combine.
	Filters []any
	// Sort contains the MongoDB sort document.
	Sort any
	// Limit bounds the number of returned documents.
	Limit *int64
	// Skip offsets the returned documents.
	Skip *int64
	// AllowAllDocuments permits unfiltered write operations.
	AllowAllDocuments bool
}

// Option configures a store operation.
type Option func(*QueryConfig)

// WithFilter adds a typed filter. Multiple filters are combined with $or.
func WithFilter[F any](filter F) Option {
	return func(config *QueryConfig) {
		config.Filters = append(config.Filters, filter)
	}
}

// WithSort sets the typed MongoDB sort document.
func WithSort[S any](sort S) Option {
	return func(config *QueryConfig) {
		config.Sort = sort
	}
}

// WithLimit limits the number of documents returned by FindMany or Stream.
func WithLimit(limit int64) Option {
	return func(config *QueryConfig) {
		config.Limit = &limit
	}
}

// WithSkip skips documents before returning query results.
func WithSkip(skip int64) Option {
	return func(config *QueryConfig) {
		config.Skip = &skip
	}
}

// WithAllDocuments explicitly permits a patch or delete operation without a
// filter.
func WithAllDocuments() Option {
	return func(config *QueryConfig) {
		config.AllowAllDocuments = true
	}
}
