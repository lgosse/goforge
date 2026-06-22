package httpmiddlewares

import "net/http"

type middlewareOptions struct {
	excludes map[string]struct{}
	includes map[string]struct{}
}

func (o *middlewareOptions) shouldExclude(r *http.Request) bool {
	if o.excludes != nil {
		if _, ok := o.excludes[r.Pattern]; ok {
			return true
		}
	}
	if o.includes != nil {
		if _, ok := o.includes[r.Pattern]; !ok {
			return true
		}
	}
	return false
}

type middlewareOption func(*middlewareOptions)

func WithMuxPatternExclusion(patterns ...string) middlewareOption {
	return func(opts *middlewareOptions) {
		if opts.excludes == nil {
			opts.excludes = make(map[string]struct{})
		}
		for _, pattern := range patterns {
			opts.excludes[pattern] = struct{}{}
		}
	}
}

func WithMuxPatternInclusion(patterns ...string) middlewareOption {
	return func(opts *middlewareOptions) {
		if opts.includes == nil {
			opts.includes = make(map[string]struct{})
		}
		for _, pattern := range patterns {
			opts.includes[pattern] = struct{}{}
		}
	}
}
