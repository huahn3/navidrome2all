package nativeapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/jukebox"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
	"github.com/navidrome/navidrome/tests"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Jukebox Outputs Endpoints", func() {
	var ds *tests.MockDataStore
	var api *Router
	var router http.Handler
	var admin model.User
	var origEnabled bool
	var origOutputs []conf.JukeboxOutputDevice

	BeforeEach(func() {
		origEnabled = conf.Server.Jukebox.Enabled
		origOutputs = conf.Server.Jukebox.Outputs
		conf.Server.Jukebox.Enabled = true
		conf.Server.Jukebox.Outputs = []conf.JukeboxOutputDevice{
			{ID: "nas", Name: "NAS speakers", Type: "mpd", Address: "127.0.0.1:1"},
		}
		jukebox.ResetInstance()

		admin = model.User{ID: "u1", UserName: "admin", IsAdmin: true}
		ds = &tests.MockDataStore{}
		api = &Router{ds: ds}
		r := chi.NewRouter()
		r.Route("/jukebox", func(r chi.Router) { api.addJukeboxOutputsRoute(r) })
		router = r
	})

	AfterEach(func() {
		conf.Server.Jukebox.Enabled = origEnabled
		conf.Server.Jukebox.Outputs = origOutputs
		jukebox.ResetInstance()
	})

	do := func(method, target string, body []byte, u model.User) *httptest.ResponseRecorder {
		var reader io.Reader
		if body != nil {
			reader = bytes.NewReader(body)
		}
		req := httptest.NewRequest(method, target, reader)
		req = req.WithContext(request.WithUser(req.Context(), u))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	Describe("GET /jukebox/outputs", func() {
		It("lists file outputs with their source and the total count header", func() {
			w := do("GET", "/jukebox/outputs", nil, admin)
			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(w.Header().Get("X-Total-Count")).To(Equal("1"))

			var outputs []jukeboxOutputDTO
			Expect(json.Unmarshal(w.Body.Bytes(), &outputs)).To(Succeed())
			Expect(outputs).To(HaveLen(1))
			Expect(outputs[0].ID).To(Equal("nas"))
			Expect(outputs[0].Source).To(Equal("config"))
		})

		It("requires an admin user", func() {
			w := do("GET", "/jukebox/outputs", nil, model.User{ID: "u2", UserName: "joe"})
			Expect(w.Code).To(Equal(http.StatusForbidden))
		})

		It("is forbidden when the jukebox is disabled", func() {
			conf.Server.Jukebox.Enabled = false
			w := do("GET", "/jukebox/outputs", nil, admin)
			Expect(w.Code).To(Equal(http.StatusForbidden))
		})
	})

	Describe("GET /jukebox/discover", func() {
		It("requires an admin user", func() {
			w := do("GET", "/jukebox/discover", nil, model.User{ID: "u2", UserName: "joe"})
			Expect(w.Code).To(Equal(http.StatusForbidden))
		})

		It("is forbidden when the jukebox is disabled", func() {
			conf.Server.Jukebox.Enabled = false
			w := do("GET", "/jukebox/discover", nil, admin)
			Expect(w.Code).To(Equal(http.StatusForbidden))
		})
	})

	Describe("POST /jukebox/outputs", func() {
		validBody := []byte(`{"id":"xiaoai-play","name":"Redmi Xiaoai","type":"xiaomi",` +
			`"address":"192.168.1.30","token":"00112233445566778899aabbccddeeff","model":"l7a"}`)

		It("creates, persists and activates a new output", func() {
			w := do("POST", "/jukebox/outputs", validBody, admin)
			Expect(w.Code).To(Equal(http.StatusCreated))

			var created jukeboxOutputDTO
			Expect(json.Unmarshal(w.Body.Bytes(), &created)).To(Succeed())
			Expect(created.ID).To(Equal("xiaoai-play"))
			Expect(created.Source).To(Equal("ui"))

			// persisted in the property table
			stored, err := jukebox.LoadStoredOutputs(context.Background(), ds)
			Expect(err).ToNot(HaveOccurred())
			Expect(stored).To(HaveLen(1))
			Expect(stored[0].Token).To(Equal("00112233445566778899aabbccddeeff"))

			// and immediately selectable
			m := jukebox.GetInstance()
			Expect(m.StoredOutputs()).To(HaveLen(1))
			ids := []string{}
			for _, d := range m.Devices() {
				ids = append(ids, d.ID)
			}
			Expect(ids).To(ContainElement("xiaoai-play"))
		})

		It("rejects invalid configurations", func() {
			w := do("POST", "/jukebox/outputs", []byte(`{"id":"bad id","name":"x","type":"xiaomi","address":"a"}`), admin)
			Expect(w.Code).To(Equal(http.StatusBadRequest))

			w = do("POST", "/jukebox/outputs", []byte(`{"id":"x","name":"x","type":"sonos","address":"a"}`), admin)
			Expect(w.Code).To(Equal(http.StatusBadRequest))

			w = do("POST", "/jukebox/outputs", []byte(`{"id":"x","name":"x","type":"xiaomi","address":"a","token":"short"}`), admin)
			Expect(w.Code).To(Equal(http.StatusBadRequest))
		})

		It("rejects duplicate ids", func() {
			w := do("POST", "/jukebox/outputs", []byte(`{"id":"nas","name":"x","type":"mpd","address":"a:1"}`), admin)
			Expect(w.Code).To(Equal(http.StatusBadRequest))
		})

		Describe("PUT /jukebox/outputs/{id}", func() {
			It("updates a stored output", func() {
				Expect(do("POST", "/jukebox/outputs",
					[]byte(`{"id":"kitchen","name":"Kitchen","type":"dlna","address":"192.168.1.40:1400"}`), admin).Code).
					To(Equal(http.StatusCreated))

				w := do("PUT", "/jukebox/outputs/kitchen",
					[]byte(`{"id":"kitchen","name":"Kitchen (new)","type":"dlna","address":"192.168.1.41:1400"}`), admin)
				Expect(w.Code).To(Equal(http.StatusOK))

				stored, err := jukebox.LoadStoredOutputs(context.Background(), ds)
				Expect(err).ToNot(HaveOccurred())
				Expect(stored).To(HaveLen(1))
				Expect(stored[0].Name).To(Equal("Kitchen (new)"))
				Expect(stored[0].Address).To(Equal("192.168.1.41:1400"))
			})

			It("creates a stored override when updating a file-configured output", func() {
				w := do("PUT", "/jukebox/outputs/nas",
					[]byte(`{"id":"nas","name":"NAS (override)","type":"mpd","address":"127.0.0.1:2"}`), admin)
				Expect(w.Code).To(Equal(http.StatusOK))

				stored, err := jukebox.LoadStoredOutputs(context.Background(), ds)
				Expect(err).ToNot(HaveOccurred())
				Expect(stored).To(HaveLen(1))

				// the override wins and is reported as ui-managed
				w = do("GET", "/jukebox/outputs/nas", nil, admin)
				Expect(w.Code).To(Equal(http.StatusOK))
				var dto jukeboxOutputDTO
				Expect(json.Unmarshal(w.Body.Bytes(), &dto)).To(Succeed())
				Expect(dto.Name).To(Equal("NAS (override)"))
				Expect(dto.Source).To(Equal("ui"))
			})

			It("returns not found for unknown outputs", func() {
				w := do("PUT", "/jukebox/outputs/nope",
					[]byte(`{"id":"nope","name":"x","type":"mpd","address":"a:1"}`), admin)
				Expect(w.Code).To(Equal(http.StatusNotFound))
			})
		})

		Describe("DELETE /jukebox/outputs/{id}", func() {
			It("deletes a stored output", func() {
				Expect(do("POST", "/jukebox/outputs",
					[]byte(`{"id":"kitchen","name":"Kitchen","type":"dlna","address":"192.168.1.40:1400"}`), admin).Code).
					To(Equal(http.StatusCreated))

				w := do("DELETE", "/jukebox/outputs/kitchen", nil, admin)
				Expect(w.Code).To(Equal(http.StatusOK))

				stored, err := jukebox.LoadStoredOutputs(context.Background(), ds)
				Expect(err).ToNot(HaveOccurred())
				Expect(stored).To(BeEmpty())
				Expect(jukebox.GetInstance().StoredOutputs()).To(BeEmpty())
			})

			It("refuses to delete a file-configured output", func() {
				w := do("DELETE", "/jukebox/outputs/nas", nil, admin)
				Expect(w.Code).To(Equal(http.StatusBadRequest))
			})

			It("returns not found for unknown outputs", func() {
				w := do("DELETE", "/jukebox/outputs/nope", nil, admin)
				Expect(w.Code).To(Equal(http.StatusNotFound))
			})
		})
	})

})
