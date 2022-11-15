package gorequests

import (
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"encoding/json"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type Request struct {
	httpreq *http.Request
	Header  *http.Header
	Client  *http.Client
}

type Response struct {
	Res        *http.Response
	content    []byte
	text       string
	Req        *Request
	Status     string // e.g. "200 OK"
	StatusCode int    // e.g. 200
	Header     http.Header
}

type Header map[string]string
type Params map[string]string
type Data map[string]string
type Json map[string]string
type Files map[string]string // name ,filename
type SetTimeout int
type Proxy string
type Cookie map[string]string

// {username,password}
type Auth []string

func Requests() *Request {

	req := new(Request)

	req.httpreq = &http.Request{
		Method:     "GET",
		Header:     make(http.Header),
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
	}
	// 指针传递后边直接用req.Header
	req.Header = &req.httpreq.Header
	req.httpreq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/97.0.4690.70 Safari/537.36")

	req.Client = &http.Client{}

	return req
}

func (req *Request) Send(method string, origurl string, args ...interface{}) (resp *Response, err error) {

	req.httpreq.Method = method
	thisTransport := &http.Transport{}

	params := map[string]string{}
	datas := map[string]string{}
	files := []map[string]string{}

	for _, arg := range args {
		switch a := arg.(type) {
		case Header:
			for k, v := range a {
				req.Header.Set(k, v)
			}
		case Params:
			params = a
		case Data:
			datas = a
		case Json:
			// set default
			// 先判断传入的header有没有Content-Type
			if req.Header.Get("Content-Type") == "" {
				req.Header.Set("Content-Type", "application/json")
			}
			jsonstr, _ := json.Marshal(a)
			req.setBodyRawBytes(ioutil.NopCloser(strings.NewReader(string(jsonstr))))
		case Files:
			files = append(files, a)
		case Auth:
			// a{username,password}
			req.httpreq.SetBasicAuth(a[0], a[1])
		case string:
			//set default
			if req.Header.Get("Content-Type") == "" {
				req.Header.Set("Content-Type", "application/json")
			}
			req.setBodyRawBytes(ioutil.NopCloser(strings.NewReader(a)))
		case SetTimeout:
			// 最容易使用的是http.Client的Timeout字段。它涵盖了从拨号（如果连接不被重用）到读取body的整个交换过程
			// but大文件下载读取body容易超时
			// req.Client.Timeout = time.Duration(a * SetTimeout(time.Second))
			// 修改为限制建立连接超时
			thisTransport.DialContext = (&net.Dialer{
				Timeout: time.Duration(a * SetTimeout(time.Second)), //连接超时时间
			}).DialContext
		case Proxy:
			proxy, _ := url.Parse(string(a))
			thisTransport.Proxy = http.ProxyURL(proxy)
			thisTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		case Cookie:
			// header里的cookie为最高优先级设置
			if req.Header.Get("Cookie") == "" {
				for k, v := range a {
					cookie1 := &http.Cookie{Name: k, Value: v}
					req.httpreq.AddCookie(cookie1)
				}
			}
		}

	}
	req.Client.Transport = thisTransport
	disturl, err := buildURLParams(origurl, params)
	if err != nil {
		return nil, err
	}

	if len(files) > 0 {
		req.buildFilesAndForms(files, datas)

	} else if len(datas) > 0 {
		//set default
		if req.Header.Get("Content-Type") == "" {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		Forms := req.buildForms(datas)
		req.setBodyBytes(Forms)
	}
	URL, err := url.Parse(disturl)
	if err != nil {
		return nil, err
	}
	req.httpreq.URL = URL

	res, err := req.Client.Do(req.httpreq)

	// clear post param
	req.httpreq.Body = nil
	req.httpreq.GetBody = nil
	req.httpreq.ContentLength = 0

	if err != nil {
		return nil, err
	}

	resp = &Response{}
	resp.Res = res
	resp.Req = req
	resp.Header = resp.Res.Header
	resp.Status = resp.Res.Status
	resp.StatusCode = resp.Res.StatusCode
	resp.Content()
	defer res.Body.Close()
	return resp, nil
}

// only set forms
func (req *Request) setBodyBytes(Forms url.Values) {
	// maybe
	data := Forms.Encode()
	req.httpreq.Body = ioutil.NopCloser(strings.NewReader(data))
	req.httpreq.ContentLength = int64(len(data))
}

// only set forms
func (req *Request) setBodyRawBytes(read io.ReadCloser) {
	req.httpreq.Body = read
}

// upload file and form
// build to body format
func (req *Request) buildFilesAndForms(files []map[string]string, datas map[string]string) {

	//handle file multipart

	var b bytes.Buffer
	w := multipart.NewWriter(&b)

	for _, file := range files {
		for k, v := range file {
			part, err := w.CreateFormFile(k, v)
			if err != nil {
				panic(err)
			}
			file := openFile(v)
			_, err = io.Copy(part, file)
			if err != nil {
				panic(err)
			}
		}
	}

	for k, v := range datas {
		w.WriteField(k, v)
	}

	w.Close()
	// set file header
	req.httpreq.Body = ioutil.NopCloser(bytes.NewReader(b.Bytes()))
	req.httpreq.ContentLength = int64(b.Len())
	req.Header.Set("Content-Type", w.FormDataContentType())
}

// build post Form data
func (req *Request) buildForms(datas map[string]string) (Forms url.Values) {
	Forms = url.Values{}
	for key, value := range datas {
		Forms.Add(key, value)
	}
	return Forms
}

func (req *Request) ReqCookies() []*http.Cookie {
	return req.httpreq.Cookies()
}

func (req *Request) Close() {
	req.httpreq.Close = true
}

func (resp *Response) Content() []byte {

	var err error

	if len(resp.content) > 0 {
		return resp.content
	}

	var Body = resp.Res.Body
	if resp.Res.Header.Get("Content-Encoding") == "gzip" && resp.Req.Header.Get("Accept-Encoding") != "" {
		reader, err := gzip.NewReader(Body)
		if err != nil {
			return nil
		}
		Body = reader
	}

	resp.content, err = ioutil.ReadAll(Body)
	if err != nil {
		return nil
	}

	return resp.content
}

func (resp *Response) Text() string {
	if resp.content == nil {
		resp.Content()
	}
	resp.text = string(resp.content)
	return resp.text
}

func (resp *Response) Json(v interface{}) error {
	if resp.content == nil {
		resp.Content()
	}
	return json.Unmarshal(resp.content, v)
}

func (resp *Response) SaveFile(filename string) error {
	if resp.content == nil {
		resp.Content()
	}
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write(resp.content)
	f.Sync()

	return err
}

func (resp *Response) Cookies() (cookies []*http.Cookie) {
	return resp.Res.Cookies()
}

func (resp *Response) Location() (*url.URL, error) {
	return resp.Res.Location()
}
