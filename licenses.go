package main

import _ "embed"

//go:embed THIRD_PARTY_LICENSES.txt
var dependencyLicenses string

//go:embed LICENSE
var wrapperLicense string

//go:embed NOTICE
var distributionNotice string
