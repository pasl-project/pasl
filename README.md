# PASL - Personalized Accounts & Safebox Ledger

[![Build status](https://ci.appveyor.com/api/projects/status/eeu8sdiakgqqtan3/branch/master?svg=true)](https://ci.appveyor.com/project/pasl-project/pasl/branch/master)
[![Build Status](https://travis-ci.org/pasl-project/pasl.svg?branch=master)](https://travis-ci.org/pasl-project/pasl)
[![Coverage Status](https://coveralls.io/repos/github/pasl-project/pasl/badge.svg)](https://coveralls.io/github/pasl-project/pasl)

PASL is a cryptocurrency based around the Safebox blockhain with strong security and easy to remember addresses (account numbers) like 0-10, 5-70 or 54321-26.

Copyright (c) 2018 PASL Project

# Compiling PASL from source

## Dependencies

* Go >= 1.10 <https://golang.org/doc/install>

## Checkout project's repository

```bash
go get -u -v github.com/pasl-project/pasl
```

## Build and install

```bash
go install github.com/pasl-project/pasl
```

Binaries will be installed in `$GOPATH/bin/` by default
