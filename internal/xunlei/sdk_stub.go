//go:build !xunlei_sdk

package xunlei

func New() Client { return unavailableClient{} }

type unavailableClient struct{}
func (unavailableClient) Init(string,string,string) error { return ErrSDKNotBuilt }
func (unavailableClient) Login(string) error { return ErrSDKNotBuilt }
func (unavailableClient) CreateP2SP(string,string,string)(uint64,error){ return 0,ErrSDKNotBuilt }
func (unavailableClient) Start(uint64) error{return ErrSDKNotBuilt}
func (unavailableClient) Stop(uint64) error{return ErrSDKNotBuilt}
func (unavailableClient) Delete(uint64,bool) error{return ErrSDKNotBuilt}
func (unavailableClient) State(uint64)(TaskState,error){return TaskState{},ErrSDKNotBuilt}
func (unavailableClient) Close() error{return nil}
