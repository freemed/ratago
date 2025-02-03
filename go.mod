module github.com/freemed/ratago

go 1.22

toolchain go1.23.2

replace (
	github.com/freemed/gokogiri/help => ../gokogiri/help
	github.com/freemed/gokogiri/html => ../gokogiri/html
	github.com/freemed/gokogiri/xml => ../gokogiri/xml
	github.com/freemed/gokogiri/xpath => ../gokogiri/xpath
	github.com/freemed/ratago => ./
	github.com/freemed/ratago/xslt => ./xslt
)

require (
	github.com/freemed/gokogiri/xml v0.0.0-20230628164547-0f93de0487ac
	github.com/freemed/kowhai v0.0.0-20150515033021-5b6ea3150fcb
	github.com/freemed/ratago/xslt v0.0.0-20230724152402-3a0c7faa982f
	github.com/smartystreets/goconvey v1.8.1
)

require (
	github.com/freemed/gokogiri/help v0.0.0-20230628164547-0f93de0487ac // indirect
	github.com/freemed/gokogiri/util v0.0.0-20230628164547-0f93de0487ac // indirect
	github.com/freemed/gokogiri/xpath v0.0.0-20230628164547-0f93de0487ac // indirect
	github.com/gopherjs/gopherjs v1.17.2 // indirect
	github.com/jtolds/gls v4.20.0+incompatible // indirect
	github.com/smarty/assertions v1.15.0 // indirect
)
