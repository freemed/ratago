module github.com/freemed/ratago

go 1.24

toolchain go1.24.3

replace (
	github.com/freemed/gokogiri/help => ../gokogiri/help
	github.com/freemed/gokogiri/html => ../gokogiri/html
	github.com/freemed/gokogiri/xml => ../gokogiri/xml
	github.com/freemed/gokogiri/xpath => ../gokogiri/xpath
	github.com/freemed/ratago => ./
	github.com/freemed/ratago/xslt => ./xslt
)

require (
	github.com/freemed/gokogiri/xml v0.0.0-20250402180648-1e651eb8ffcd
	github.com/freemed/kowhai v0.0.0-20150515033021-5b6ea3150fcb
	github.com/freemed/ratago/xslt v0.0.0-20250203231425-016f1ea48158
	github.com/smartystreets/goconvey v1.8.1
)

require (
	github.com/freemed/gokogiri/help v0.0.0-20250402180648-1e651eb8ffcd // indirect
	github.com/freemed/gokogiri/util v0.0.0-20250402180648-1e651eb8ffcd // indirect
	github.com/freemed/gokogiri/xpath v0.0.0-20250402180648-1e651eb8ffcd // indirect
	github.com/gopherjs/gopherjs v1.17.2 // indirect
	github.com/jtolds/gls v4.20.0+incompatible // indirect
	github.com/smarty/assertions v1.15.0 // indirect
)
