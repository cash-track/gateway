package client

import "github.com/valyala/fasthttp"

type MockClient struct {
	list  []*MockExpectation
	index uint
}

func (m *MockClient) next() *MockExpectation {
	i := m.index
	m.index++
	return m.list[i]
}

func (m *MockClient) Do(req *fasthttp.Request, resp *fasthttp.Response) error {
	e := m.next()
	if e == nil {
		return nil
	}

	if e.respFn != nil {
		e.respFn(resp)
	}

	e.req = &fasthttp.Request{}
	req.CopyTo(e.req)
	return e.err
}

func (m *MockClient) Expect(at uint) *MockExpectation {
	if m.list == nil {
		m.list = make([]*MockExpectation, 1)
	}
	if len(m.list) <= int(at) {
		m.list = append(m.list, &MockExpectation{})
	}
	if m.list[at] == nil {
		m.list[at] = &MockExpectation{}
	}
	return m.list[at]
}

func (m *MockClient) GetRequestAt(at uint) *fasthttp.Request {
	if m.list == nil || m.list[at] == nil {
		return nil
	}
	return m.list[at].req
}

func (m *MockClient) GetRequest() *fasthttp.Request {
	return m.GetRequestAt(0)
}

func (m *MockClient) ReturnError(err error) *MockExpectation {
	m.Expect(0)
	m.list[0].err = err
	return m.list[0]
}

func (m *MockClient) MockResponse(fn func(*fasthttp.Response)) *MockExpectation {
	m.Expect(0)
	m.list[0].respFn = fn
	return m.list[0]
}

type MockExpectation struct {
	respFn func(*fasthttp.Response)
	req    *fasthttp.Request
	err    error
}

func (m *MockExpectation) GetRequest() *fasthttp.Request {
	return m.req
}

func (m *MockExpectation) ReturnError(err error) *MockExpectation {
	m.err = err
	return m
}

func (m *MockExpectation) MockResponse(fn func(*fasthttp.Response)) *MockExpectation {
	m.respFn = fn
	return m
}
