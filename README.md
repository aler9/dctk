
# dctoolkit

[![GoDoc](https://godoc.org/github.com/aler9/dctoolkit?status.svg)](https://godoc.org/github.com/aler9/dctoolkit)
[![Go Report Card](https://goreportcard.com/badge/github.com/aler9/dctoolkit)](https://goreportcard.com/report/github.com/aler9/dctoolkit)
[![Build Status](https://travis-ci.org/aler9/dctoolkit.svg?branch=master)](https://travis-ci.org/aler9/dctoolkit)

dctoolkit implements the client part of the Direct Connect peer-to-peer system (ADC and NMDC protocols) in the Go programming language. It includes:
* a [**library**](#library), that allows the creation of clients capable of interacting with hubs and other clients;
* a series of [**command line utilities**](#command-line-utilities) that make use of the library.

Direct Connect is semi-centralized peer-to-peer system in which peers connect to servers (hubs) and exchange textual messages and files. Files are indexed by computing their Tiger Tree Hash (TTH), provided by users through their file list, and searchable on a hub-basis. There exist two variants, one based on the traditional NMDC protocol (NeoModus Direct Connect) and the other based on the newer ADC protocol (Advanced Direct Connect).

This project is based on the [**go-dc**](https://github.com/direct-connect/go-dc) project, that provides a base layer for building DC-related software.

## Features

* ADC and NMDC transparent protocol support
* **Active** and **passive** mode
* **Hub**: connection with configurable try count, password authentication, keepalive, compression, encryption
* **Chat**: bidirectional public and private chat
* **File search**: by name or TTH, reply to requests
* **File download**: by name or TTH, full or partial, on ram or disk, multiple in parallel, compression, encryption, configurable download slots, validation via TTH, client fingerprint validation
* **File upload**: upload from personal share, asynchronous file indexing system, file list generation and serving, compression, encryption, configurable upload slots, tthl extension support, client fingerprint validation
* Examples provided for every feature, comprehensive test suite, continuous integration

Note: this project uses the rolling release development model, as it is used in a production environment which requires the latest updates. The public API may suffer minor changes. The master branch is to be considered stable.

## Library

#### Installation

Go &ge; 1.12 is required, and modules must be enabled (there must be a `go.mod` file in your project folder, that can be created with the command `go mod init main`). To install the library, it is enough to write its name in the import section of the source files that will use it. Go will take care of downloading the needed files:
```go
import (
    dctk "github.com/aler9/dctoolkit"
)
```

#### Examples

* [connection_active](examples/01connection_active.go)
* [connection_passive](examples/02connection_passive.go)
* [chat_public](examples/03chat_public.go)
* [chat_private](examples/04chat_private.go)
* [search](examples/05search.go)
* [share](examples/06share.go)
* [magnet](examples/07magnet.go)
* [download_list](examples/08download_list.go)
* [download_all_lists](examples/09download_all_lists.go)
* [download_file](examples/10download_file.go)
* [download_file_on_disk](examples/11download_file_on_disk.go)
* [download_file_from_search](examples/12download_file_from_search.go)
* [download_file_from_list](examples/13download_file_from_list.go)
* [download_directory_from_list](examples/14download_directory_from_list.go)
* [download_streaming](examples/15download_streaming.go)

#### Documentation

https://godoc.org/github.com/aler9/dctoolkit

#### Testing

If you want to edit this library and test the results, you can you automated tests with:
```
make test
```

## Command-line utilities

#### Installation

Go &ge; 1.12 is required. Download, compile and install the binaries with:
```
go get github.com/aler9/dctoolkit/cmd/...
```

#### Usage

```
dc-tth [<flags>] <filepath>

Compute the Tiger Tree Hash (TTH) of a given file.
```

```
dc-search --hub=HUB --nick=NICK [<flags>] <query>

Search files and directories by name in a given hub.
```

```
dc-download --hub=HUB --nick=NICK --outdir=OUTDIR [<flags>] <user> <fpath>

Download a file or a directory from a user in a given hub.
```

```
dc-share --hub=HUB --nick=NICK [<flags>] <share>

Share a directory in a given hub.
```

## Links

Base library
* https://github.com/direct-connect/go-dc

Protocol references
* (ADC) https://adc.dcbase.org/Protocol
* (ADC) https://adc.dcbase.org/Extensions
* (NMDC) http://nmdc.sourceforge.net/Versions/NMDC-1.3.html
* (NMDC) https://web.archive.org/web/20160412113951/http://wiki.gusari.org/index.php?title=Main_Page

Hubs
* (NMDC) http://www.ptokax.org
* (NMDC) https://github.com/Verlihub/verlihub
* (ADC) http://adchpp.sourceforge.net/
* (ADC) https://luadch.github.io/
* (ADC) https://www.uhub.org/
* (NMDC & ADC) https://sourceforge.net/projects/flexhubdc/
* (NMDC & ADC) http://rushub.org/
* (NMDC & ADC) https://github.com/direct-connect/go-dcpp

Clients
* (NMDC) https://dev.yorhel.nl/ncdc
* (NMDC & ADC) https://github.com/eiskaltdcpp/eiskaltdcpp
* (NMDC & ADC) http://dcplusplus.sourceforge.net/
* (NMDC & ADC) https://www.airdcpp.net/
* (NMDC & ADC) https://www.apexdc.net/
* (NMDC) http://jucy.eu/
* (NMDC & ADC) https://launchpad.net/linuxdcpp
* (NMDC) https://github.com/lilyball/dcbot

Other libraries
* [Go] (NMDC & ADC) https://git.ivysaur.me/code.ivysaur.me/libnmdc
* [Go] (ADC) https://github.com/ehmry/go-adc
* [Go] (ADC) https://github.com/seoester/adcl
* [Python] (NMDC) https://github.com/ashishgaurav13/nmdc
* [Python] (NMDC) http://pydc.sourceforge.net/
* [Python] (ADC) https://pypi.org/project/pyADC/

Inspired by
* https://godoc.org/github.com/anacrolix/torrent
