package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // photo sizes
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-sky/internal/astrometry"
	"github.com/abjelosevic88/pocket-homelab-sky/internal/store"
)

// ---- photos of the sky ----
//
// Photos are kept as files under DATA_DIR/photos/<user>/ and belong to
// their owner only. "What's in this photo?" asks astrometry.net (with the
// person's own free key: exact, names every star and object) or Pocket AI
// (a description, told what was up at the time).

const maxPhoto = 40 << 20

type solveState struct {
	Engine  string             `json:"engine"` // astrometry | ai
	State   string             `json:"state"`  // queued | solving | success | failure
	SubID   int                `json:"subid,omitempty"`
	JobID   int                `json:"jobid,omitempty"`
	Checked time.Time          `json:"checked"`
	Result  *astrometry.Result `json:"result,omitempty"`
	Text    string             `json:"text,omitempty"`
	Objects []string           `json:"objects,omitempty"`
	Error   string             `json:"error,omitempty"`
}

type photoView struct {
	store.Photo
	URL   string      `json:"url"`
	Solve *solveState `json:"solve"`
}

func viewPhoto(p store.Photo) photoView {
	v := photoView{Photo: p, URL: "/api/v1/photos/" + strconv.FormatInt(p.ID, 10) + "/file"}
	if p.Solve != "" {
		var st solveState
		if json.Unmarshal([]byte(p.Solve), &st) == nil {
			v.Solve = &st
		}
	}
	return v
}

func (s *Server) photoPath(uid int64, name string) string {
	return filepath.Join(s.cfg.DataDir, "photos", strconv.FormatInt(uid, 10), name)
}

func (s *Server) handleListPhotos(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ps, err := s.store.ListPhotos(ctx, actorID(ctx), int64(queryInt(r, "observation")))
	if err != nil {
		s.fail(w, err, "photos")
		return
	}
	out := make([]photoView, 0, len(ps))
	for _, p := range ps {
		out = append(out, viewPhoto(p))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleUploadPhoto takes a photo (multipart "file", with observation_id,
// object_key and caption fields).
func (s *Server) handleUploadPhoto(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	r.Body = http.MaxBytesReader(w, r.Body, maxPhoto+1<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "a photo of up to 40 MB")
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "choose a photo")
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxPhoto+1))
	if err != nil || len(data) > maxPhoto {
		writeErr(w, http.StatusRequestEntityTooLarge, "a photo of up to 40 MB")
		return
	}
	mime := http.DetectContentType(data)
	ext := map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp", "image/gif": ".gif"}[mime]
	if ext == "" {
		if strings.HasSuffix(strings.ToLower(hdr.Filename), ".heic") {
			writeErr(w, http.StatusBadRequest, "HEIC photos can't be shown in the browser; on an iPhone choose Most Compatible in Settings → Camera → Formats, or share the photo as JPEG")
			return
		}
		writeErr(w, http.StatusBadRequest, "a JPEG, PNG or WebP photo")
		return
	}
	p := &store.Photo{Mime: mime, Size: int64(len(data)), Caption: strings.TrimSpace(r.FormValue("caption")), Key: strings.TrimSpace(r.FormValue("object_key"))}
	if cfg, _, err := image.DecodeConfig(bytes.NewReader(data)); err == nil {
		p.Width, p.Height = cfg.Width, cfg.Height
	}
	if oid, _ := strconv.ParseInt(r.FormValue("observation_id"), 10, 64); oid > 0 {
		if _, err := s.store.GetObservation(ctx, uid, oid); err != nil {
			writeErr(w, http.StatusBadRequest, "unknown log entry")
			return
		}
		p.ObservationID = &oid
	}
	var rb [8]byte
	_, _ = rand.Read(rb[:])
	p.File = time.Now().UTC().Format("20060102-150405") + "-" + hex.EncodeToString(rb[:]) + ext
	path := s.photoPath(uid, p.File)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		s.fail(w, err, "photo")
		return
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		s.fail(w, err, "photo")
		return
	}
	if err := s.store.AddPhoto(ctx, uid, p); err != nil {
		_ = os.Remove(path)
		s.fail(w, err, "photo")
		return
	}
	writeJSON(w, http.StatusCreated, viewPhoto(*p))
}

func (s *Server) handlePhotoFile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	p, err := s.store.GetPhoto(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "photo")
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Header().Set("Content-Type", p.Mime)
	http.ServeFile(w, r, s.photoPath(uid, p.File))
}

func (s *Server) handleGetPhoto(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	p, err := s.store.GetPhoto(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "photo")
		return
	}
	s.pollSolve(ctx, uid, p)
	writeJSON(w, http.StatusOK, viewPhoto(*p))
}

func (s *Server) handleUpdatePhoto(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	p, err := s.store.GetPhoto(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "photo")
		return
	}
	var in struct {
		Caption       *string `json:"caption"`
		Key           *string `json:"key"`
		ObservationID *int64  `json:"observation_id"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	trimP(&p.Caption, in.Caption)
	trimP(&p.Key, in.Key)
	if in.ObservationID != nil {
		p.ObservationID = nil
		if *in.ObservationID > 0 {
			if _, err := s.store.GetObservation(ctx, uid, *in.ObservationID); err != nil {
				writeErr(w, http.StatusBadRequest, "unknown log entry")
				return
			}
			p.ObservationID = in.ObservationID
		}
	}
	if len(p.Caption) > 2000 {
		writeErr(w, http.StatusBadRequest, "a short caption")
		return
	}
	if err := s.store.UpdatePhoto(ctx, uid, p); err != nil {
		s.fail(w, err, "photo")
		return
	}
	writeJSON(w, http.StatusOK, viewPhoto(*p))
}

func (s *Server) handleDeletePhoto(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	p, err := s.store.GetPhoto(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "photo")
		return
	}
	if err := s.store.DeletePhoto(ctx, uid, p.ID); err != nil {
		s.fail(w, err, "photo")
		return
	}
	_ = os.Remove(s.photoPath(uid, p.File))
	w.WriteHeader(http.StatusNoContent)
}

// handleSolvePhoto starts "what's in this photo?".
func (s *Server) handleSolvePhoto(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	p, err := s.store.GetPhoto(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "photo")
		return
	}
	st, _ := s.store.GetSettings(ctx, uid)
	data, err := os.ReadFile(s.photoPath(uid, p.File))
	if err != nil {
		s.fail(w, err, "photo")
		return
	}
	var ss solveState
	if st.AstrometryKey != "" && r.URL.Query().Get("engine") != "ai" {
		c := astrometry.New(st.AstrometryKey, s.cfg.UserAgent)
		ss = solveState{Engine: "astrometry", State: "queued", Checked: time.Now()}
		session, err := c.Login(ctx)
		if err == nil {
			ss.SubID, err = c.Upload(ctx, session, p.File, data)
		}
		if err != nil {
			ss.State, ss.Error = "failure", err.Error()
		}
	} else {
		ss = s.describePhoto(ctx, uid, st, p, data)
	}
	b, _ := json.Marshal(ss)
	p.Solve = string(b)
	if err := s.store.UpdatePhoto(ctx, uid, p); err != nil {
		s.fail(w, err, "photo")
		return
	}
	writeJSON(w, http.StatusOK, viewPhoto(*p))
}

// pollSolve checks astrometry.net for a photo still being solved (every 8 s at most).
func (s *Server) pollSolve(ctx context.Context, uid int64, p *store.Photo) {
	if p.Solve == "" {
		return
	}
	var ss solveState
	if json.Unmarshal([]byte(p.Solve), &ss) != nil || ss.Engine != "astrometry" || ss.State == "success" || ss.State == "failure" || time.Since(ss.Checked) < 8*time.Second {
		return
	}
	st, _ := s.store.GetSettings(ctx, uid)
	c := astrometry.New(st.AstrometryKey, s.cfg.UserAgent)
	ss.Checked = time.Now()
	job, err := c.Status(ctx, ss.SubID)
	switch {
	case err != nil:
		ss.Error = err.Error()
	case job.State == "success":
		ss.JobID = job.ID
		if res, err := c.Fetch(ctx, job.ID); err == nil {
			ss.State, ss.Result, ss.Error = "success", res, ""
		} else {
			ss.Error = err.Error()
		}
	case job.State == "failure":
		ss.State, ss.JobID, ss.Error = "failure", job.ID, "astrometry.net could not find the stars' pattern (too few stars, too blurred, or a daytime photo)"
	default:
		ss.State, ss.JobID = job.State, job.ID
	}
	b, _ := json.Marshal(ss)
	p.Solve = string(b)
	_ = s.store.UpdatePhoto(ctx, uid, p)
}

// describePhoto asks the AI what the photo shows, telling it what was up then.
func (s *Server) describePhoto(ctx context.Context, uid int64, st store.Settings, p *store.Photo, data []byte) solveState {
	ss := solveState{Engine: "ai", Checked: time.Now()}
	if !st.AIPhotos {
		ss.State, ss.Error = "failure", "sending photos to the AI is switched off (Settings → AI)"
		return ss
	}
	cl, used, why := s.aiFor(ctx, uid, st)
	if cl == nil {
		ss.State, ss.Error = "failure", firstNonEmpty(why, "no AI is set up (Settings → AI), and no astrometry.net key")
		return ss
	}
	// what was up when and where the photo was taken (the log entry, else now at home)
	context := ""
	t := time.Now()
	var wh *where
	if p.ObservationID != nil {
		if o, err := s.store.GetObservation(ctx, uid, *p.ObservationID); err == nil {
			if o.StartedAt != "" {
				if tt, err := time.ParseInLocation("2006-01-02T15:04", o.StartedAt, time.Local); err == nil {
					t = tt
				}
			}
			if o.LocationID != nil {
				if l, err := s.store.GetLocation(ctx, uid, *o.LocationID); err == nil {
					w := whereLoc(l)
					wh = &w
				}
			}
		}
	}
	if wh == nil {
		if l, err := s.store.HomeLocation(ctx, uid); err == nil {
			w := whereLoc(l)
			wh = &w
		}
	}
	if wh != nil {
		var up []string
		for _, b := range bodiesAt(wh.Obs, t) {
			if b.Up && b.Key != "sun" {
				up = append(up, fmt.Sprintf("%s (%.0f° up in the %s, magnitude %.1f)", b.Name, b.Alt, b.Dir, b.Mag))
			}
		}
		context = fmt.Sprintf("It was probably taken around %s at %s. Up then: %s.", t.In(wh.TZ).Format("2 Jan 2006 15:04"), wh.Name, strings.Join(up, "; "))
	}
	prompt := "This is a photo of the sky. " + context + ` Say plainly what it shows: the Moon, planets, bright stars, constellations or star clusters you can recognise, the Milky Way, satellites or planes, clouds. Only name what you can really see. Answer as JSON {"summary": "2 or 3 short sentences", "objects": ["names"]}.`
	img := []aiImage{{Mime: p.Mime, Data: data}}
	var text string
	var err error
	if used == "pocket" {
		text, err = s.pocketVision(ctx, uid, st, aiVoice, prompt, img)
	} else {
		text, err = s.openaiVision(ctx, st, aiVoice, prompt, img)
	}
	if err != nil {
		ss.State, ss.Error = "failure", err.Error()
		return ss
	}
	var out struct {
		Summary string   `json:"summary"`
		Objects []string `json:"objects"`
	}
	text = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(text), "```json"), "```"), "```"))
	if i := strings.IndexByte(text, '{'); i >= 0 {
		text = text[i:]
	}
	if json.Unmarshal([]byte(text), &out) != nil {
		out.Summary = text
	}
	ss.State, ss.Text, ss.Objects = "success", out.Summary, out.Objects
	return ss
}

// ---- pictures for the AI ----

type aiImage struct {
	Mime string
	Data []byte
}

func (s *Server) pocketVision(ctx context.Context, uid int64, st store.Settings, system, prompt string, images []aiImage) (string, error) {
	u, err := s.store.GetUser(ctx, uid)
	if err != nil {
		return "", err
	}
	tok, err := s.core.ActAs(ctx, u.Email, "ai", core.InferenceScope)
	if err != nil {
		return "", fmt.Errorf("Pocket AI: %w", err)
	}
	body := map[string]any{"system": system, "prompt": prompt, "json": true}
	if st.AIPocketModel != "" {
		body["model"] = st.AIPocketModel
	}
	var imgs []map[string]string
	for _, im := range images {
		imgs = append(imgs, map[string]string{"mime": im.Mime, "data": base64.StdEncoding.EncodeToString(im.Data)})
	}
	body["images"] = imgs
	raw, _ := json.Marshal(body)
	ctx, cancel := context.WithTimeout(ctx, 170*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(tok.URL, "/")+"/api/v1/inference", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	res, err := (&http.Client{Timeout: 170 * time.Second}).Do(req)
	if err != nil {
		return "", fmt.Errorf("Pocket AI: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(&e)
		return "", fmt.Errorf("Pocket AI HTTP %d: %s", res.StatusCode, e.Error)
	}
	var out core.InferenceReply
	if err := json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&out); err != nil {
		return "", err
	}
	if strings.TrimSpace(out.Text) == "" {
		return "", errors.New("Pocket AI returned no answer")
	}
	return out.Text, nil
}

func (s *Server) openaiVision(ctx context.Context, st store.Settings, system, prompt string, images []aiImage) (string, error) {
	base := strings.TrimRight(firstNonEmpty(st.OpenAIBaseURL, "https://api.openai.com/v1"), "/")
	model := firstNonEmpty(st.OpenAIModel, "gpt-4o-mini")
	parts := []map[string]any{{"type": "text", "text": prompt}}
	for _, im := range images {
		parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:" + im.Mime + ";base64," + base64.StdEncoding.EncodeToString(im.Data), "detail": "high"}})
	}
	body := map[string]any{"model": model, "response_format": map[string]string{"type": "json_object"},
		"messages": []map[string]any{{"role": "system", "content": system}, {"role": "user", "content": parts}}}
	raw, _ := json.Marshal(body)
	ctx, cancel := context.WithTimeout(ctx, 150*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+st.OpenAIKey)
	req.Header.Set("Content-Type", "application/json")
	res, err := (&http.Client{Timeout: 150 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("AI HTTP %d", res.StatusCode)
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(b, &out); err != nil || len(out.Choices) == 0 {
		return "", errors.New("the AI returned no answer")
	}
	return out.Choices[0].Message.Content, nil
}
