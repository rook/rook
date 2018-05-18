/*
Copyright 2017 The Rook Authors. All rights reserved.

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

package flexvolume

type MockFlexvolumeController struct {
	MockAttach                    func(attachOpts AttachOptions, devicePath *string) error
	MockDetach                    func(detachOpts AttachOptions, _ *struct{} /* void reply */) error
	MockDetachForce               func(detachOpts AttachOptions, _ *struct{} /* void reply */) error
	MockRemoveAttachmentObject    func(detachOpts AttachOptions, safeToDetach *bool) error
	MockLog                       func(message LogMessage, _ *struct{} /* void reply */) error
	MockGetAttachInfoFromMountDir func(mountDir string, attachOptions *AttachOptions) error
}

func (m *MockFlexvolumeController) Attach(attachOpts AttachOptions, devicePath *string) error {
	if m.MockAttach != nil {
		return m.MockAttach(attachOpts, devicePath)
	}
	return nil
}

func (m *MockFlexvolumeController) Detach(detachOpts AttachOptions, _ *struct{} /* void reply */) error {
	if m.MockDetach != nil {
		return m.MockDetach(detachOpts, nil)
	}
	return nil
}

func (m *MockFlexvolumeController) DetachForce(detachOpts AttachOptions, _ *struct{} /* void reply */) error {
	if m.MockDetachForce != nil {
		return m.MockDetachForce(detachOpts, nil)
	}
	return nil
}

func (m *MockFlexvolumeController) RemoveAttachmentObject(detachOpts AttachOptions, safeToDetach *bool) error {
	if m.MockRemoveAttachmentObject != nil {
		return m.MockRemoveAttachmentObject(detachOpts, safeToDetach)
	}
	return nil
}

func (m *MockFlexvolumeController) Log(message LogMessage, _ *struct{} /* void reply */) error {
	if m.MockLog != nil {
		return m.MockLog(message, nil)
	}
	return nil
}

func (m *MockFlexvolumeController) GetAttachInfoFromMountDir(mountDir string, attachOptions *AttachOptions) error {
	if m.MockGetAttachInfoFromMountDir != nil {
		return m.MockGetAttachInfoFromMountDir(mountDir, attachOptions)
	}
	return nil
}
