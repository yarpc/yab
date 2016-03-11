exception ThriftException {}

service Simple {
  void foo()
  i32 bar()
  void thriftEx() throws (1: ThriftException ex)
}
