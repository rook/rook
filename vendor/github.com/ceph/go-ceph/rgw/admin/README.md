# Prerequisites

You must create an admin user like so:

```
radosgw-admin user create --uid admin --display-name "Admin User" --caps "buckets=*;users=*;usage=read;metadata=read;zone=read --access-key=AKIAIOSFODNN7EXAMPLE --secret-key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
```

Then use the `access_key` and `secret_key` for authentication.

Snippet usage example:

```golang
package main

import (
    "github.com/ceph/go-ceph/rgw/admin"
)

func main() {
    // Generate a connection object
    co, err := admin.New("http://192.168.1.1", "AKIAIOSFODNN7EXAMPLE", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", nil)
    if err != nil {
        panic(err)
    }

    // Get the "admin" user
    user, err := co.GetUser(context.Background(), admin.User{ID: "admin"})
    if err != nil {
        panic(err)
    }

    // Print the user display name
    fmt.Println(user.DisplayName)
}
```
