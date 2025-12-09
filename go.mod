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
	github.com/freemed/gokogiri/xml v0.0.0-20251130225105-1c0457d97f4b
	github.com/freemed/ratago/xslt v0.0.0-20251130224444-ff8869104a5d
)

require (
	github.com/freemed/gokogiri/help v0.0.0-20251130225105-1c0457d97f4b // indirect
	github.com/freemed/gokogiri/util v0.0.0-20251130225105-1c0457d97f4b // indirect
	github.com/freemed/gokogiri/xpath v0.0.0-20251130225105-1c0457d97f4b // indirect
)
