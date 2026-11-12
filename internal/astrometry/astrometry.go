// Package astrometry asks nova.astrometry.net what a photo of the sky shows:
// it finds the stars' pattern, says exactly where the picture points, how
// big it is and how it is turned, and names the stars and objects in it.
// It needs the person's own free API key (nova.astrometry.net → My Profile).
package astrometry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Base is the service, a variable so tests can use a fake one.
var Base = "https://nova.astrometry.net"

// Client talks to astrometry.net with one API key.
type Client struct {
	Key  string
	HTTP *http.Client
	UA   string
}

// New makes a client.
func New(key, ua string) *Client {
	return &Client{Key: strings.TrimSpace(key), HTTP: &http.Client{Timeout: 90 * time.Second}, UA: ua}
}

func (c *Client) post(ctx context.Context, path string, body io.Reader, ctype string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, Base+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", ctype)
	req.Header.Set("User-Agent", c.UA)
	return c.do(req, out)
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, Base+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.UA)
	return c.do(req, out)
}

func (c *Client) do(req *http.Request, out any) error {
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("astrometry.net: HTTP %d", res.StatusCode)
	}
	var st struct {
		Status       string `json:"status"`
		ErrorMessage string `json:"errormessage"`
	}
	_ = json.Unmarshal(b, &st)
	if st.Status == "error" {
		return errors.New("astrometry.net: " + st.ErrorMessage)
	}
	return json.Unmarshal(b, out)
}

func form(v any) io.Reader {
	b, _ := json.Marshal(v)
	return strings.NewReader("request-json=" + url.QueryEscape(string(b)))
}

// Login opens a session with the key.
func (c *Client) Login(ctx context.Context) (string, error) {
	var out struct {
		Session string `json:"session"`
	}
	if err := c.post(ctx, "/api/login", form(map[string]string{"apikey": c.Key}), "application/x-www-form-urlencoded", &out); err != nil {
		return "", err
	}
	if out.Session == "" {
		return "", errors.New("astrometry.net: the API key was not accepted")
	}
	return out.Session, nil
}

// Upload sends a photo (kept private) and returns its submission id.
func (c *Client) Upload(ctx context.Context, session, name string, data []byte) (int, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	rj, _ := json.Marshal(map[string]any{"session": session, "publicly_visible": "n", "allow_commercial_use": "n", "allow_modifications": "n"})
	_ = mw.WriteField("request-json", string(rj))
	fw, err := mw.CreateFormFile("file", name)
	if err != nil {
		return 0, err
	}
	if _, err := fw.Write(data); err != nil {
		return 0, err
	}
	_ = mw.Close()
	var out struct {
		SubID int `json:"subid"`
	}
	if err := c.post(ctx, "/api/upload", &buf, mw.FormDataContentType(), &out); err != nil {
		return 0, err
	}
	if out.SubID == 0 {
		return 0, errors.New("astrometry.net did not take the photo")
	}
	return out.SubID, nil
}

// Job is how far the solving is: the job id once there is one, and its state.
type Job struct {
	ID    int    `json:"id"`
	State string `json:"state"` // queued | solving | success | failure
}

// Status checks a submission.
func (c *Client) Status(ctx context.Context, subID int) (Job, error) {
	var sub struct {
		Jobs []*int `json:"jobs"`
	}
	if err := c.get(ctx, "/api/submissions/"+strconv.Itoa(subID), &sub); err != nil {
		return Job{}, err
	}
	if len(sub.Jobs) == 0 || sub.Jobs[0] == nil {
		return Job{State: "queued"}, nil
	}
	j := Job{ID: *sub.Jobs[0]}
	var st struct {
		Status string `json:"status"`
	}
	if err := c.get(ctx, "/api/jobs/"+strconv.Itoa(j.ID), &st); err != nil {
		return j, err
	}
	j.State = st.Status
	if j.State == "" {
		j.State = "solving"
	}
	return j, nil
}

// Calibration is where the photo points: the centre (J2000 degrees), the
// scale (arc seconds a pixel), how it is turned (degrees east of north of the
// picture's "up") and whether it is mirrored (parity).
type Calibration struct {
	RA          float64 `json:"ra"`
	Dec         float64 `json:"dec"`
	Radius      float64 `json:"radius"`
	PixScale    float64 `json:"pixscale"`
	Orientation float64 `json:"orientation"`
	Parity      float64 `json:"parity"`
}

// Annotation is a named star or object at a spot in the photo.
type Annotation struct {
	Names  []string `json:"names"`
	Type   string   `json:"type"`
	X      float64  `json:"x"`
	Y      float64  `json:"y"`
	Radius float64  `json:"radius"`
}

// Result is what a solved photo shows.
type Result struct {
	Calibration Calibration  `json:"calibration"`
	Objects     []string     `json:"objects"`
	Annotations []Annotation `json:"annotations"`
}

// Fetch gets a solved job's calibration, objects and annotations.
func (c *Client) Fetch(ctx context.Context, jobID int) (*Result, error) {
	r := &Result{}
	if err := c.get(ctx, fmt.Sprintf("/api/jobs/%d/calibration/", jobID), &r.Calibration); err != nil {
		return nil, err
	}
	var obj struct {
		Objects []string `json:"objects_in_field"`
	}
	if err := c.get(ctx, fmt.Sprintf("/api/jobs/%d/objects_in_field/", jobID), &obj); err == nil {
		r.Objects = obj.Objects
	}
	var ann struct {
		Annotations []struct {
			Names  []string `json:"names"`
			Type   string   `json:"type"`
			X      float64  `json:"pixelx"`
			Y      float64  `json:"pixely"`
			Radius float64  `json:"radius"`
		} `json:"annotations"`
	}
	if err := c.get(ctx, fmt.Sprintf("/api/jobs/%d/annotations/", jobID), &ann); err == nil {
		for _, a := range ann.Annotations {
			r.Annotations = append(r.Annotations, Annotation{Names: a.Names, Type: a.Type, X: a.X, Y: a.Y, Radius: a.Radius})
		}
	}
	return r, nil
}
