package server

import (
	"errors"
	"io"
	"mime"
	"net/http"

	"github.com/wahhheee/yanyi/internal/pkg/errs"
	"github.com/wahhheee/yanyi/internal/work"
)

func (s *Server) listWorks(w http.ResponseWriter, r *http.Request) error {
	list, err := s.d.Works.List(r.Context())
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, list)
	return nil
}

func (s *Server) getWork(w http.ResponseWriter, r *http.Request) error {
	d, err := s.d.Works.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, d)
	return nil
}

func (s *Server) listWorkSaves(w http.ResponseWriter, r *http.Request) error {
	list, err := s.d.Saves.ListByWork(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, list)
	return nil
}

// importWork：POST /works/import —— 导入第一步：解析与校验（不写入作品库），返回 work.ImportPreview。
//
// 请求体是 multipart/form-data，字段按顺序读取（流式处理，不整体缓存请求体）：
//
//	source=zip：name（可选，zip 文件名），然后一个 file（zip 包）
//	source=dir：name（可选，所选文件夹名），然后每个文件一对 path（相对路径，含文件夹名）+ file
//
// 浏览器上传时 multipart 的 filename 会被标准库截成最后一段（防路径穿越），所以文件夹的相对路径放在单独的 path 字段。
func (s *Server) importWork(w http.ResponseWriter, r *http.Request) error {
	mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	switch mt {
	case "multipart/form-data":
	case "application/json":
		return errs.NotImplemented("从 GitHub、链接或本机路径导入（M6-1 / M6-2）")
	default:
		return errs.Invalid("请以 multipart/form-data 上传 zip 包或文件夹")
	}
	lim := s.d.Imports.Limits()
	r.Body = http.MaxBytesReader(w, r.Body, lim.UploadLimit())
	p, err := s.readImport(r)
	if err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			// 上限比作品总量多留了 zip 头与表单分隔的余量，对玩家只说作品总量
			return errs.New(errs.CodeWorkInvalid, "上传的内容过大（作品总体积上限 %d MiB）", lim.MaxTotal>>20)
		}
		return err
	}
	writeJSON(w, http.StatusOK, p)
	return nil
}

func (s *Server) readImport(r *http.Request) (*work.ImportPreview, error) {
	mr, err := r.MultipartReader()
	if err != nil {
		return nil, errs.Invalid("上传内容格式不正确：%v", err)
	}
	ctx := r.Context()
	var source, name, path string
	var folder *work.FolderUpload
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeInvalidArgument, "上传内容格式不正确")
		}
		switch part.FormName() {
		case "source":
			source, err = formValue(part)
		case "name":
			name, err = formValue(part)
		case "path":
			path, err = formValue(part)
		case "file":
			switch source {
			case "zip":
				if name == "" {
					name = part.FileName()
				}
				return s.d.Imports.PreviewZip(ctx, name, part) // zip 只有一个文件，其后的内容不再读取
			case "dir":
				if path == "" {
					return nil, errs.Invalid("文件缺少相对路径（path 字段应在 file 之前）")
				}
				if folder == nil {
					folder = s.d.Imports.NewFolderUpload()
				}
				err, path = folder.Add(path, part), ""
			default:
				return nil, errs.Invalid("source 应为 zip 或 dir，且写在文件之前")
			}
		default:
			return nil, errs.Invalid("未知的表单字段：%s", part.FormName())
		}
		if err != nil {
			return nil, err
		}
	}
	if source == "dir" && folder != nil {
		return s.d.Imports.PreviewFolder(ctx, folder, name)
	}
	return nil, errs.Invalid("没有收到要导入的文件")
}

// formValue 读取一个短文本字段。
func formValue(part io.Reader) (string, error) {
	const maxLen = 4096
	b, err := io.ReadAll(io.LimitReader(part, maxLen+1))
	if err != nil {
		return "", errs.Wrap(err, errs.CodeInvalidArgument, "上传内容格式不正确")
	}
	if len(b) > maxLen {
		return "", errs.Invalid("表单字段过长")
	}
	return string(b), nil
}

// confirmImport：POST /works/import/{task}/confirm —— 导入第二步：写入作品库，返回 work.ImportResult。
func (s *Server) confirmImport(w http.ResponseWriter, r *http.Request) error {
	res, err := s.d.Imports.Confirm(r.Context(), r.PathValue("task"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, res)
	return nil
}

// cancelImport：DELETE /works/import/{task} —— 放弃导入，删除暂存文件（关闭导入对话框时调用；不存在也返回成功）。
func (s *Server) cancelImport(w http.ResponseWriter, r *http.Request) error {
	s.d.Imports.Cancel(r.Context(), r.PathValue("task"))
	w.WriteHeader(http.StatusNoContent)
	return nil
}
