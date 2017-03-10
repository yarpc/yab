struct QueryLocation {
  1: required double latitude
  2: required double longitude
  3: optional i32 cityId
  4: optional string message
}

service Simple {
  void foo(1: QueryLocation location)
}
