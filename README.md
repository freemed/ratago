ratago
======

[![Build Status](https://travis-ci.org/freemed/ratago.svg?branch=master)](https://travis-ci.org/freemed/ratago)
[![codecov](https://codecov.io/gh/freemed/ratago/branch/master/graph/badge.svg)](https://codecov.io/gh/freemed/ratago)
[![Go Report Card](https://goreportcard.com/badge/github.com/freemed/ratago)](https://goreportcard.com/report/github.com/freemed/ratago)
[![GoDoc](https://godoc.org/github.com/freemed/ratago?status.svg)](https://godoc.org/github.com/freemed/ratago)

Ratago is a (mostly-compliant) implementation of an XSLT 1.0 processor written in Go and released under an MIT license.

Currently it should be seen as experimental - it lacks full compliance with the spec. It has been run successfully on a number of scripts of moderate complexity as of the 0.4-pre release.

The test suite is derived from the test suite used by the libxslt library written by Daniel Veillard. See http://xmlsoft.org/XSLT/ for details on libxslt.

TODO
----

There are several tasks remaining to reach full compliance. Until these tasks are complete the API is subject to change.

* Implement xsl:decimal-format and format-number.
* Ensure that errors are properly progogated in Go fashion.

