package subsonic

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	. "github.com/Masterminds/squirrel"
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/consts"
	"github.com/navidrome/navidrome/core/lyrics"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/resources"
	"github.com/navidrome/navidrome/server/imghttp"
	"github.com/navidrome/navidrome/server/subsonic/responses"
	"github.com/navidrome/navidrome/utils/gravatar"
	"github.com/navidrome/navidrome/utils/req"
)

func (api *Router) GetAvatar(w http.ResponseWriter, r *http.Request) (*responses.Subsonic, error) {
	if !conf.Server.EnableGravatar {
		return api.getPlaceHolderAvatar(w, r)
	}
	p := req.Params(r)
	username, err := p.String("username")
	if err != nil {
		return nil, err
	}
	ctx := r.Context()
	u, err := api.ds.User(ctx).FindByUsername(username)
	if err != nil {
		return nil, err
	}
	if u.Email == "" {
		log.Warn(ctx, "User needs an email for gravatar to work", "username", username)
		return api.getPlaceHolderAvatar(w, r)
	}
	http.Redirect(w, r, gravatar.Url(u.Email, 0), http.StatusFound) //nolint:gosec // URL is not constructed from user input
	return nil, nil
}

func (api *Router) getPlaceHolderAvatar(w http.ResponseWriter, r *http.Request) (*responses.Subsonic, error) {
	f, err := resources.FS().Open(consts.PlaceholderAvatar)
	if err != nil {
		log.Error(r, "Image not found", err)
		return nil, newError(responses.ErrorDataNotFound, "Avatar image not found")
	}
	defer f.Close()
	_, _ = io.Copy(w, f)

	return nil, nil
}

func (api *Router) GetCoverArt(w http.ResponseWriter, r *http.Request) (*responses.Subsonic, error) {
	// If context is already canceled, discard request without further processing
	if r.Context().Err() != nil {
		return nil, nil //nolint:nilerr
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	p := req.Params(r)
	id, _ := p.String("id")
	size := p.IntOr("size", 0)
	square := p.BoolOr("square", false)

	img, err := api.artwork.GetOrPlaceholder(ctx, id, size, square)
	switch {
	case errors.Is(err, context.Canceled):
		return nil, nil
	case errors.Is(err, model.ErrNotFound):
		log.Warn(r, "Couldn't find coverArt", "id", id, err)
		return nil, newError(responses.ErrorDataNotFound, "Artwork not found")
	case err != nil:
		log.Error(r, "Error retrieving coverArt", "id", id, err)
		return nil, err
	}

	defer img.Close()

	artID, _ := model.ParseArtworkID(id)
	if imghttp.WriteImageHeaders(w, r, img, artID.Hash) {
		return nil, nil
	}
	cnt, err := io.Copy(w, img)
	if err != nil {
		log.Warn(ctx, "Error sending image", "count", cnt, err)
	}

	return nil, err
}

func (api *Router) GetLyrics(r *http.Request) (*responses.Subsonic, error) {
	p := req.Params(r)
	artist, _ := p.String("artist")
	title, _ := p.String("title")
	response := newResponse()
	lyricsResponse := responses.Lyrics{}
	response.Lyrics = &lyricsResponse
	structuredLyrics, err := api.lyrics.GetLyricsByArtistTitle(r.Context(), artist, title)
	if err != nil {
		return nil, err
	}

	bilingual, _ := p.Bool("bilingual")
	if bilingual {
		targetLang, _ := p.String("lang")
		if targetLang == "" {
			targetLang = "zh-CN"
		}
		mediaFiles, err := api.ds.MediaFile(r.Context()).GetAll(model.QueryOptions{
			Filters: And{
				Eq{"missing": false},
				Eq{"title": title},
			},
			Max: 1,
		})
		if err == nil && len(mediaFiles) > 0 {
			transSvc := lyrics.GetTranslationService(api.ds)
			if trans, err := transSvc.GetCachedTranslation(mediaFiles[0].ID, targetLang); err == nil && trans != nil && trans.BilingualLRC != "" {
				lyricsResponse.Artist = artist
				lyricsResponse.Title = title
				lyricsResponse.Value = trans.BilingualLRC + "\n"
				return response, nil
			}
		}
	}

	mainLyric, ok := structuredLyrics.Main()
	if !ok {
		return response, nil
	}

	lyricsResponse.Artist = artist
	lyricsResponse.Title = title

	var lyricsText strings.Builder
	for _, line := range mainLyric.Line {
		lyricsText.WriteString(line.Value + "\n")
	}
	lyricsResponse.Value = lyricsText.String()

	return response, nil
}

func (api *Router) GetLyricsBySongId(r *http.Request) (*responses.Subsonic, error) {
	p := req.Params(r)
	id, err := p.String("id")
	if err != nil {
		return nil, err
	}

	mediaFile, err := api.ds.MediaFile(r.Context()).Get(id)
	if err != nil {
		return nil, err
	}

	structuredLyrics, err := api.lyrics.GetLyrics(r.Context(), mediaFile)
	if err != nil {
		return nil, err
	}

	enhanced, _ := p.Bool("enhanced")
	bilingual, _ := p.Bool("bilingual")
	translate, _ := p.Bool("translate")
	targetLang, _ := p.String("lang")
	if targetLang == "" {
		targetLang = "zh-CN"
	}

	transSvc := lyrics.GetTranslationService(api.ds)
	var trans *lyrics.LyricTranslation
	if translate || bilingual {
		trans, _ = transSvc.TranslateSong(r.Context(), mediaFile, targetLang, false)
	} else {
		trans, _ = transSvc.GetCachedTranslation(mediaFile.ID, targetLang)
	}

	if trans != nil && len(trans.Lines) > 0 {
		if bilingual && !enhanced {
			for i := range structuredLyrics {
				if structuredLyrics[i].IsMainKind() {
					for j := range structuredLyrics[i].Line {
						if j < len(trans.Lines) && trans.Lines[j].Translation != "" {
							structuredLyrics[i].Line[j].Value += "\n" + trans.Lines[j].Translation
						}
					}
					break
				}
			}
		} else {
			transLines := make([]model.Line, len(trans.Lines))
			for i, l := range trans.Lines {
				transLines[i] = model.Line{
					Start: l.Start,
					End:   l.End,
					Value: l.Translation,
				}
			}
			structuredLyrics = append(structuredLyrics, model.Lyrics{
				Kind:   model.LyricKindTranslation,
				Lang:   trans.TargetLang,
				Synced: true,
				Line:   transLines,
			})
		}
	}

	response := newResponse()
	response.LyricsList = buildLyricsList(mediaFile, structuredLyrics, enhanced)

	return response, nil
}
