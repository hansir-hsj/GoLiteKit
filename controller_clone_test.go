package golitekit

import (
	"context"
	"sync"
	"testing"
)

// SimpleController tests basic field copying
type SimpleController struct {
	BaseController[NoBody]
	Name    string
	Age     int
	Score   float64
	Enabled bool
}

func (c *SimpleController) Serve(ctx context.Context) error {
	return nil
}

// PointerController tests pointer field deep copy
type PointerController struct {
	BaseController[NoBody]
	Data *TestData
}

type TestData struct {
	Value  int
	Name   string
	Nested *NestedData
}

type NestedData struct {
	ID int
}

func (c *PointerController) Serve(ctx context.Context) error {
	return nil
}

// SyncController tests that sync primitives are skipped
type SyncController struct {
	BaseController[NoBody]
	Name string
	mu   sync.Mutex
	rw   sync.RWMutex
	wg   sync.WaitGroup
	once sync.Once
}

func (c *SyncController) Serve(ctx context.Context) error {
	return nil
}

// InterfaceController tests interface{} deep copy
type InterfaceController struct {
	BaseController[NoBody]
	Data    interface{}
	Handler interface{}
}

func (c *InterfaceController) Serve(ctx context.Context) error {
	return nil
}

// MapController tests map deep copy
type MapController struct {
	BaseController[NoBody]
	SimpleMap  map[string]int
	PointerMap map[string]*TestData
	StructMap  map[string]TestData
}

func (c *MapController) Serve(ctx context.Context) error {
	return nil
}

// SliceController tests slice deep copy
type SliceController struct {
	BaseController[NoBody]
	Numbers  []int
	Pointers []*TestData
	Structs  []TestData
}

func (c *SliceController) Serve(ctx context.Context) error {
	return nil
}

// ChannelController tests that channels are skipped
type ChannelController struct {
	BaseController[NoBody]
	Name string
	Ch   chan int
}

func (c *ChannelController) Serve(ctx context.Context) error {
	return nil
}

// FuncController tests that functions are shared
type FuncController struct {
	BaseController[NoBody]
	Handler func() int
}

func (c *FuncController) Serve(ctx context.Context) error {
	return nil
}

// EmbeddedSyncController tests embedded struct with sync primitives
type EmbeddedSyncController struct {
	BaseController[NoBody]
	Name     string
	Embedded EmbeddedWithMutex
}

type EmbeddedWithMutex struct {
	Value int
	mu    sync.Mutex
}

func (c *EmbeddedSyncController) Serve(ctx context.Context) error {
	return nil
}

// ComplexController tests a combination of all types
type ComplexController struct {
	BaseController[NoBody]
	Name       string
	Data       *TestData
	Items      map[string]*TestData
	List       []*TestData
	Any        interface{}
	mu         sync.Mutex
	Ch         chan int
	OnComplete func()
}

func (c *ComplexController) Serve(ctx context.Context) error {
	return nil
}

// ============================================================================
// Test Cases
// ============================================================================

func TestCloneController_SimpleFields(t *testing.T) {
	src := &SimpleController{
		Name:    "test",
		Age:     25,
		Score:   98.5,
		Enabled: true,
	}

	cloned := CloneController(src)
	dst := cloned.(*SimpleController)

	// Check values are copied
	if dst.Name != src.Name {
		t.Errorf("Name not copied: got %s, want %s", dst.Name, src.Name)
	}
	if dst.Age != src.Age {
		t.Errorf("Age not copied: got %d, want %d", dst.Age, src.Age)
	}
	if dst.Score != src.Score {
		t.Errorf("Score not copied: got %f, want %f", dst.Score, src.Score)
	}
	if dst.Enabled != src.Enabled {
		t.Errorf("Enabled not copied: got %v, want %v", dst.Enabled, src.Enabled)
	}

	// Verify they are independent
	dst.Name = "modified"
	dst.Age = 30
	if src.Name == dst.Name {
		t.Error("Modifying clone should not affect source")
	}
	if src.Age == dst.Age {
		t.Error("Modifying clone should not affect source")
	}
}

func TestCloneController_NilSource(t *testing.T) {
	var src *SimpleController = nil
	cloned := CloneController(src)
	if cloned != nil {
		t.Error("Cloning nil should return nil")
	}
}

func TestCloneController_PointerFields(t *testing.T) {
	src := &PointerController{
		Data: &TestData{
			Value: 100,
			Name:  "original",
			Nested: &NestedData{
				ID: 999,
			},
		},
	}

	cloned := CloneController(src)
	dst := cloned.(*PointerController)

	// Check values are copied
	if dst.Data == nil {
		t.Fatal("Data pointer should not be nil")
	}
	if dst.Data.Value != src.Data.Value {
		t.Errorf("Data.Value not copied: got %d, want %d", dst.Data.Value, src.Data.Value)
	}
	if dst.Data.Name != src.Data.Name {
		t.Errorf("Data.Name not copied: got %s, want %s", dst.Data.Name, src.Data.Name)
	}

	// Check nested pointer
	if dst.Data.Nested == nil {
		t.Fatal("Nested pointer should not be nil")
	}
	if dst.Data.Nested.ID != src.Data.Nested.ID {
		t.Errorf("Nested.ID not copied: got %d, want %d", dst.Data.Nested.ID, src.Data.Nested.ID)
	}

	// Verify deep copy - pointers should be different
	if dst.Data == src.Data {
		t.Error("Data pointer should be different (deep copy)")
	}
	if dst.Data.Nested == src.Data.Nested {
		t.Error("Nested pointer should be different (deep copy)")
	}

	// Verify independence
	dst.Data.Value = 200
	dst.Data.Nested.ID = 888
	if src.Data.Value == 200 {
		t.Error("Modifying clone Data.Value should not affect source")
	}
	if src.Data.Nested.ID == 888 {
		t.Error("Modifying clone Nested.ID should not affect source")
	}
}

func TestCloneController_NilPointerField(t *testing.T) {
	src := &PointerController{
		Data: nil,
	}

	cloned := CloneController(src)
	dst := cloned.(*PointerController)

	if dst.Data != nil {
		t.Error("Nil pointer should remain nil")
	}
}

func TestCloneController_SyncPrimitivesSkipped(t *testing.T) {
	src := &SyncController{
		Name: "test",
	}

	// Lock the mutex in source to verify it's not copied
	src.mu.Lock()
	defer src.mu.Unlock()

	cloned := CloneController(src)
	dst := cloned.(*SyncController)

	// Name should be copied
	if dst.Name != src.Name {
		t.Errorf("Name not copied: got %s, want %s", dst.Name, src.Name)
	}

	// The cloned mutex should be at zero value (unlocked)
	// Use TryLock (Go 1.18+) to test without blocking
	if !dst.mu.TryLock() {
		t.Error("Cloned mutex should be at zero value (unlocked)")
	} else {
		dst.mu.Unlock()
	}
}

func TestCloneController_InterfaceWithPointer(t *testing.T) {
	originalData := &TestData{Value: 42, Name: "interface-test"}
	src := &InterfaceController{
		Data: originalData,
	}

	cloned := CloneController(src)
	dst := cloned.(*InterfaceController)

	// Check value is copied
	if dst.Data == nil {
		t.Fatal("Interface Data should not be nil")
	}

	dstData, ok := dst.Data.(*TestData)
	if !ok {
		t.Fatal("Interface Data should be *TestData")
	}

	if dstData.Value != originalData.Value {
		t.Errorf("Interface Data.Value not copied: got %d, want %d", dstData.Value, originalData.Value)
	}

	// Verify deep copy - pointers should be different
	if dstData == originalData {
		t.Error("Interface containing pointer should be deep copied")
	}

	// Verify independence
	dstData.Value = 100
	if originalData.Value == 100 {
		t.Error("Modifying cloned interface data should not affect source")
	}
}

func TestCloneController_InterfaceWithPrimitive(t *testing.T) {
	src := &InterfaceController{
		Data: "hello",
	}

	cloned := CloneController(src)
	dst := cloned.(*InterfaceController)

	if dst.Data != "hello" {
		t.Errorf("Interface with string not copied: got %v, want %s", dst.Data, "hello")
	}
}

func TestCloneController_MapDeepCopy(t *testing.T) {
	src := &MapController{
		SimpleMap: map[string]int{
			"a": 1,
			"b": 2,
		},
		PointerMap: map[string]*TestData{
			"x": {Value: 10, Name: "x-data"},
			"y": {Value: 20, Name: "y-data"},
		},
		StructMap: map[string]TestData{
			"s": {Value: 30, Name: "s-data"},
		},
	}

	cloned := CloneController(src)
	dst := cloned.(*MapController)

	// Check SimpleMap
	if len(dst.SimpleMap) != len(src.SimpleMap) {
		t.Errorf("SimpleMap length mismatch: got %d, want %d", len(dst.SimpleMap), len(src.SimpleMap))
	}
	if dst.SimpleMap["a"] != 1 || dst.SimpleMap["b"] != 2 {
		t.Error("SimpleMap values not copied correctly")
	}

	// Verify SimpleMap independence
	dst.SimpleMap["a"] = 100
	if src.SimpleMap["a"] == 100 {
		t.Error("Modifying clone SimpleMap should not affect source")
	}

	// Check PointerMap - values should be deep copied
	if dst.PointerMap["x"] == src.PointerMap["x"] {
		t.Error("PointerMap values should be deep copied (different pointers)")
	}
	if dst.PointerMap["x"].Value != 10 {
		t.Errorf("PointerMap value not copied: got %d, want %d", dst.PointerMap["x"].Value, 10)
	}

	// Verify PointerMap independence
	dst.PointerMap["x"].Value = 999
	if src.PointerMap["x"].Value == 999 {
		t.Error("Modifying clone PointerMap value should not affect source")
	}

	// Check StructMap
	if dst.StructMap["s"].Value != 30 {
		t.Errorf("StructMap value not copied: got %d, want %d", dst.StructMap["s"].Value, 30)
	}
}

func TestCloneController_NilMap(t *testing.T) {
	src := &MapController{
		SimpleMap:  nil,
		PointerMap: nil,
	}

	cloned := CloneController(src)
	dst := cloned.(*MapController)

	if dst.SimpleMap != nil {
		t.Error("Nil map should remain nil")
	}
	if dst.PointerMap != nil {
		t.Error("Nil map should remain nil")
	}
}

func TestCloneController_SliceDeepCopy(t *testing.T) {
	src := &SliceController{
		Numbers: []int{1, 2, 3},
		Pointers: []*TestData{
			{Value: 10, Name: "p1"},
			{Value: 20, Name: "p2"},
		},
		Structs: []TestData{
			{Value: 30, Name: "s1"},
		},
	}

	cloned := CloneController(src)
	dst := cloned.(*SliceController)

	// Check Numbers slice
	if len(dst.Numbers) != len(src.Numbers) {
		t.Errorf("Numbers length mismatch: got %d, want %d", len(dst.Numbers), len(src.Numbers))
	}

	// Verify Numbers independence
	dst.Numbers[0] = 100
	if src.Numbers[0] == 100 {
		t.Error("Modifying clone Numbers should not affect source")
	}

	// Check Pointers slice - elements should be deep copied
	if dst.Pointers[0] == src.Pointers[0] {
		t.Error("Pointer slice elements should be deep copied")
	}
	if dst.Pointers[0].Value != 10 {
		t.Errorf("Pointer slice element value not copied: got %d, want %d", dst.Pointers[0].Value, 10)
	}

	// Verify Pointers independence
	dst.Pointers[0].Value = 999
	if src.Pointers[0].Value == 999 {
		t.Error("Modifying clone Pointers element should not affect source")
	}

	// Check Structs slice
	if dst.Structs[0].Value != 30 {
		t.Errorf("Struct slice element value not copied: got %d, want %d", dst.Structs[0].Value, 30)
	}
}

func TestCloneController_NilSlice(t *testing.T) {
	src := &SliceController{
		Numbers:  nil,
		Pointers: nil,
	}

	cloned := CloneController(src)
	dst := cloned.(*SliceController)

	if dst.Numbers != nil {
		t.Error("Nil slice should remain nil")
	}
	if dst.Pointers != nil {
		t.Error("Nil slice should remain nil")
	}
}

func TestCloneController_ChannelSkipped(t *testing.T) {
	ch := make(chan int, 1)
	src := &ChannelController{
		Name: "test",
		Ch:   ch,
	}

	cloned := CloneController(src)
	dst := cloned.(*ChannelController)

	// Name should be copied
	if dst.Name != src.Name {
		t.Errorf("Name not copied: got %s, want %s", dst.Name, src.Name)
	}

	// Channel should be nil (skipped)
	if dst.Ch != nil {
		t.Error("Channel should be skipped (nil)")
	}
}

func TestCloneController_FuncShared(t *testing.T) {
	callCount := 0
	handler := func() int {
		callCount++
		return callCount
	}

	src := &FuncController{
		Handler: handler,
	}

	cloned := CloneController(src)
	dst := cloned.(*FuncController)

	// Function should be shared (same reference)
	if dst.Handler == nil {
		t.Fatal("Handler should not be nil")
	}

	// Calling either should increment the same counter
	src.Handler()
	result := dst.Handler()

	if result != 2 {
		t.Errorf("Function should be shared: got %d, want 2", result)
	}
}

func TestCloneController_EmbeddedSyncType(t *testing.T) {
	src := &EmbeddedSyncController{
		Name: "test",
		Embedded: EmbeddedWithMutex{
			Value: 42,
		},
	}

	// Lock embedded mutex
	src.Embedded.mu.Lock()
	defer src.Embedded.mu.Unlock()

	cloned := CloneController(src)
	dst := cloned.(*EmbeddedSyncController)

	// Name should be copied
	if dst.Name != src.Name {
		t.Errorf("Name not copied: got %s, want %s", dst.Name, src.Name)
	}

	// Embedded.Value should be copied
	if dst.Embedded.Value != src.Embedded.Value {
		t.Errorf("Embedded.Value not copied: got %d, want %d", dst.Embedded.Value, src.Embedded.Value)
	}

	// Embedded mutex should be at zero value
	// Use TryLock to test without blocking
	if !dst.Embedded.mu.TryLock() {
		t.Error("Embedded mutex should be at zero value (unlocked)")
	} else {
		dst.Embedded.mu.Unlock()
	}
}

func TestCloneController_ComplexController(t *testing.T) {
	completeCalled := false
	src := &ComplexController{
		Name: "complex",
		Data: &TestData{Value: 1, Name: "data"},
		Items: map[string]*TestData{
			"item1": {Value: 10},
		},
		List: []*TestData{
			{Value: 20},
		},
		Any:        &TestData{Value: 30},
		Ch:         make(chan int),
		OnComplete: func() { completeCalled = true },
	}

	src.mu.Lock()
	defer src.mu.Unlock()

	cloned := CloneController(src)
	dst := cloned.(*ComplexController)

	// Check basic field
	if dst.Name != "complex" {
		t.Errorf("Name not copied: got %s", dst.Name)
	}

	// Check pointer field is deep copied
	if dst.Data == src.Data {
		t.Error("Data should be deep copied")
	}
	if dst.Data.Value != 1 {
		t.Errorf("Data.Value not copied: got %d", dst.Data.Value)
	}

	// Check map is deep copied
	if dst.Items["item1"] == src.Items["item1"] {
		t.Error("Map values should be deep copied")
	}

	// Check slice is deep copied
	if dst.List[0] == src.List[0] {
		t.Error("Slice elements should be deep copied")
	}

	// Check interface is deep copied
	if dst.Any.(*TestData) == src.Any.(*TestData) {
		t.Error("Interface value should be deep copied")
	}

	// Check channel is skipped
	if dst.Ch != nil {
		t.Error("Channel should be skipped")
	}

	// Check function is shared
	dst.OnComplete()
	if !completeCalled {
		t.Error("Function should be shared")
	}

	// Check mutex is not copied in locked state
	// Use TryLock to test without blocking
	if !dst.mu.TryLock() {
		t.Error("Mutex should be at zero value (unlocked)")
	} else {
		dst.mu.Unlock()
	}
}

// ============================================================================
// Benchmark Tests
// ============================================================================

func BenchmarkCloneController_Simple(b *testing.B) {
	src := &SimpleController{
		Name:    "benchmark",
		Age:     25,
		Score:   98.5,
		Enabled: true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CloneController(src)
	}
}

func BenchmarkCloneController_Complex(b *testing.B) {
	src := &ComplexController{
		Name: "benchmark",
		Data: &TestData{Value: 1, Name: "data"},
		Items: map[string]*TestData{
			"item1": {Value: 10},
			"item2": {Value: 20},
		},
		List: []*TestData{
			{Value: 30},
			{Value: 40},
		},
		Any: &TestData{Value: 50},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CloneController(src)
	}
}
