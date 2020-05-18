// Copyright (c) 2020 Red Hat, Inc.

package multiclustermonitoring

import (
	"testing"
)

func TestNewDefaultObjectStorageConfigSpec(t *testing.T) {
	spec := newDefaultObjectStorageConfigSpec()

	if spec.Type != defaultObjStorageType {
		t.Errorf("Type (%v) is not the expected (%v)", spec.Type, defaultObjStorageType)
	}

	if spec.Config.Bucket != defaultObjStorageBucket {
		t.Errorf("Bucket (%v) is not the expected (%v)", spec.Config.Bucket, defaultObjStorageBucket)
	}

	if spec.Config.Endpoint != defaultObjStorageEndpoint {
		t.Errorf("Endpoint (%v) is not the expected (%v)", spec.Config.Endpoint, defaultObjStorageEndpoint)
	}

	if spec.Config.Insecure != defaultObjStorageInsecure {
		t.Errorf("Insecure (%v) is not the expected (%v)", spec.Config.Insecure, defaultObjStorageInsecure)
	}

	if spec.Config.AccessKey != defaultObjStorageAccesskey {
		t.Errorf("AccessKey (%v) is not the expected (%v)", spec.Config.AccessKey, defaultObjStorageAccesskey)
	}

	if spec.Config.SecretKey != defaultObjStorageSecretkey {
		t.Errorf("SecretKey (%v) is not the expected (%v)", spec.Config.SecretKey, defaultObjStorageSecretkey)
	}

	if spec.Config.Storage != defaultObjStorageStorage {
		t.Errorf("Storage (%v) is not the expected (%v)", spec.Config.Storage, defaultObjStorageStorage)
	}

}
