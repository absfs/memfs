# memfs - In Memory File System
The `memfs` package implements the `absfs.FileSystem` interface as a RAM backed filesystem.

## Install 

```bash
$ go get github.com/absfs/memfs
```

## Example Usage

```go
package main

import(
    "fmt"
    "os"

    "github.com/absfs/memfs"
)

func main() {
    fs, _ := memfs.NewFS() // remember kids don't ignore errors

    // Opens a file with read/write permissions in the current directory
    f, _ := fs.Create("/example.txt")  
    
    f.Write([]byte("Hello, world!"))
    f.Close()

    fs.Remove("example.txt")
}
```

## absfs
Check out the [`absfs`](https://github.com/absfs/absfs) repo for more information about the abstract filesystem interface and features like filesystem composition 

## LICENSE

This project is governed by the MIT License. See [LICENSE](https://github.com/absfs/osfs/blob/master/LICENSE)



