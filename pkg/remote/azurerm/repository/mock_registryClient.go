// Code generated by mockery v0.0.0-dev. DO NOT EDIT.

package repository

import (
	armcontainerregistry "github.com/Azure/azure-sdk-for-go/sdk/containerregistry/armcontainerregistry"
	mock "github.com/stretchr/testify/mock"
)

// mockRegistryClient is an autogenerated mock type for the registryClient type
type mockRegistryClient struct {
	mock.Mock
}

// List provides a mock function with given fields: options
func (_m *mockRegistryClient) List(options *armcontainerregistry.RegistriesListOptions) registryListAllPager {
	ret := _m.Called(options)

	var r0 registryListAllPager
	if rf, ok := ret.Get(0).(func(*armcontainerregistry.RegistriesListOptions) registryListAllPager); ok {
		r0 = rf(options)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(registryListAllPager)
		}
	}

	return r0
}