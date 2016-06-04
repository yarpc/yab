exception NotFound {}

service Foo {
	i32 bar(1: i32 arg) throws (1: NotFound notFound)
}
