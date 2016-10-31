struct QueryLocation {
  1: required double latitude
  2: required double longitude
  3: optional i32 cityId
}

service Simple {
  void foo(1: QueryLocation location)
}
