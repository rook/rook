/*
Copyright 2016 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package rgw

import (
	"encoding/json"
	"fmt"
	"path"

	etcd "github.com/coreos/etcd/client"
	ctx "golang.org/x/net/context"

	"github.com/rook/rook/pkg/cephmgr/mon"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util"
)

const (
	builtinUserKey = "admin"
	idKey          = "id"
	secretKey      = "_secret"
)

type user struct {
	Keys []keyInfo `json:"keys"`
}

type keyInfo struct {
	User      string `json:"user"`
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
}

func createBuiltinUser(context *clusterd.Context) error {
	logger.Infof("creating the built-in rgw user")
	result, err := RunAdminCommand(context,
		"user",
		"create",
		fmt.Sprintf("--uid=%s", "rookadmin"),
		fmt.Sprintf("--display-name=%s", "rook rgw builtin user"))
	if err != nil {
		return fmt.Errorf("failed to create user: %+v", err)
	}

	// Parse the creds from the json response
	var u user
	if err := json.Unmarshal([]byte(result), &u); err != nil {
		return fmt.Errorf("failed to read user info. %+v, result=%s", err, result)
	}

	if len(u.Keys) == 0 {
		return fmt.Errorf("missing keys in %s", result)
	}

	userkey := u.Keys[0]
	if userkey.AccessKey == "" || userkey.SecretKey == "" {
		return fmt.Errorf("missing user properties in %s", result)
	}

	// store the creds in etcd
	key := path.Join(mon.CephKey, ObjectStoreKey, clusterd.AppliedKey, builtinUserKey)
	if _, err := context.EtcdClient.Set(ctx.Background(), path.Join(key, idKey), userkey.AccessKey, nil); err != nil {
		return fmt.Errorf("failed to store access id. %+v", err)
	}

	if _, err := context.EtcdClient.Set(ctx.Background(), path.Join(key, secretKey), userkey.SecretKey, nil); err != nil {
		return fmt.Errorf("failed to store access id. %+v", err)
	}

	return nil
}

func GetBuiltinUserAccessInfo(etcdClient etcd.KeysAPI) (string, string, error) {
	return getUserAccessInfo(builtinUserKey, etcdClient)
}

func getUserAccessInfo(userName string, etcdClient etcd.KeysAPI) (string, string, error) {
	userKey := path.Join(mon.CephKey, ObjectStoreKey, clusterd.AppliedKey, userName)
	keys := map[string]string{
		idKey:     path.Join(userKey, idKey),
		secretKey: path.Join(userKey, secretKey),
	}

	vals, err := util.GetEtcdValues(etcdClient, keys)
	if err != nil {
		return "", "", err
	}

	return vals[idKey], vals[secretKey], nil
}
