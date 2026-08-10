module github.com/freemed/ratago

go 1.25.0

replace (
	github.com/freemed/gokogiri/help => ../gokogiri/help
	github.com/freemed/gokogiri/html => ../gokogiri/html
	github.com/freemed/gokogiri/xml => ../gokogiri/xml
	github.com/freemed/gokogiri/xpath => ../gokogiri/xpath
	github.com/freemed/ratago => ./
	github.com/freemed/ratago/xslt => ./xslt
)

require (
	github.com/freemed/gokogiri/xml v0.0.0-20260810185137-6692e94708c7
	github.com/freemed/ratago/xslt v0.0.0-20251209120218-62d49e66fc88
)

require (
	github.com/antchfx/xpath v1.3.8 // indirect
	github.com/freemed/gokogiri/xpath v0.0.0-20260810185137-6692e94708c7 // indirect
	golang.org/x/net v0.57.0 // indirect
	golang.org/x/text v0.40.0 // indirect
)
