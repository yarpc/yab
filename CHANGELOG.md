Changelog
=========

# 0.6.0

* Allow JSON to be used with non-map requests and responses.
* Expose HTTP response body on non-OK responses.
* Expose TChannel call status ("ok") and HTTP status code ("code"
  on successful responses.
* Fix ini parsing of option groups. (#82)
* Add support for TMultiplexedProtocol when using HTTP+Thrift using
  `--multiplexed-thrift`.

# 0.5.4

* Fix for benchmarking taking longer than duration at low RPS. (#73)

# 0.5.2

* Fix `--peer-list` not loading the peer list. Regression from 0.5.0. (#70)

# 0.5.0

* Support for reading default options using XDG base directories.
* Round robin peer-selection when creating connections for benchmarking.
* Allow disabling Thrift envelopes for HTTP using `--disable-thrift-envelope`.

# 0.4.0

* First beta release.
