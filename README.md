# inflight
Package inflight provides a duplicate function call suppression mechanism.

It allows multiple concurrent callers for the same key to share the result
of a single function execution, reducing redundant work and resource usage.

All types in this package are safe for concurrent use by multiple goroutines.

Very similar to [x/sync/singleflight](https://pkg.go.dev/golang.org/x/sync/singleflight), but supporting generics and using a more lightweight API (only `Group.Do` & `Group.Forget`).

It is also lock-free, if that matters (although `sync.OnceValues` and `sync.Map` use mutex internally so..).

## Dependencies
The package has two dependencies :
* [github.com/go4org/hashtriemap](https://github.com/go4org/hashtriemap) which is the internal, generic, implementation of `sync.Map` exposed in a repository by @bradfitz (with all the internal code removed/replaced)
* `github.com/stretchr/testify` for tests