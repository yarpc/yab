Changelog
=========

# 0.7.0

* Upgrade to TChannel 1.2.0 and add Open Tracing support.
* Support set constants.

# 0.6.2

* Add error rate to benchmarking output.
* Improve boolean parsing for Thrift input.
* Check the system XDG directory for a config file.
* Allow unlimited duration or requests when benchmarking. (#105)

# 0.6.1

* Improve default format detection:
  - If `-t` is specified, assume Thrift
  - Otherwise, assume JSON
* Improved support for quoted strings as JSON payloads.
* Add RPC-Encoding header for HTTP requests for compatibility
  with the latest version of YARPC.
* Any peers specified on the command-line should override all
  peer options set in the defaults.ini. (#104)

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
