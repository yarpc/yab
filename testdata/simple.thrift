exception ThriftException {}

service Simple {
  void foo()
  i32 bar()
  void thriftEx() throws (1: ThriftException ex)

  void withDefault(1: set<i32> values = [1, 2, 3])
}
