package queue

// Gate: mọi hàng RabbitMQ có người publish phải có consumer.
//
// Lỗi gốc (QA 261013 F1b, 261021): POST /exercises/:id/submit publish vào exchange "exercise" key
// "exercise.submit", hàng có khai báo nhưng KHÔNG có consumer nào — bài nộp kẹt 'pending' mãi và tin
// nhắn chất đống trong broker. Không test nào bắt được vì publish thành công trả nil.
//
// Cách làm: quét tĩnh (go/ast) mọi file .go không phải test dưới internal/ và cmd/:
//   - publish:  X.PublishMessage(ctx, exchange, key, msg) / X.PublishWithContext(ctx, exchange, key, ...)
//   - binding:  X.SetupQueue(exchange, queue, key) / X.QueueBind(queue, key, exchange, noWait, args)
//   - consumer: X.ConsumeMessages(ctx, queue, fn) / X.Consume(queue, tag, ...7 tham số) /
//     WorkerConfig{QueueName: queue} (BaseWorker consume config.QueueName)
// Đối số được phân giải từ chuỗi literal hoặc hằng chuỗi cấp package (theo tên). Lời gọi bên trong hàm
// bọc chỉ chuyển tiếp tham số (vd RabbitMQService.PublishMessage tự gọi PublishWithContext) được bỏ qua.
// Publish mà đối số KHÔNG phân giải được (không phải tham số chuyển tiếp) làm test ĐỎ — thêm publish
// động thì phải làm nó phân giải được, không được để gate mù. Consumer/binding không phân giải được thì
// không được tính, nên quét sót chỉ có thể làm test đỏ nhầm, không thể xanh nhầm.
//
// Giới hạn đã biết: gate chứng minh CÓ lời gọi consume cho hàng đó trong mã nguồn, không chứng minh
// lời gọi ấy được khởi động lúc boot (không dựng call graph).

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// knownUnconsumedQueues: hàng HÔM NAY đã có publisher mà không có consumer — lỗi thật đang treo, không
// phải giấy phép. Companion TestQueueConsumerCoverage_KnownGapsStillAccurate đỏ khi một mục được sửa
// (có consumer hoặc bỏ publisher): khi đó XOÁ mục khỏi đây, không bao giờ thêm mục mới để test xanh.
var knownUnconsumedQueues = map[string]string{
	"certificate.generate": "CertificateService.IssueCertificate publishes PDF-generation jobs; no worker consumes them",
	"video.cleanup":        "VideoProcessingService publishes cleanup commands; no consumer for video.cleanup",
	"video.results":        "VideoQueueSetup.PublishVideoResultMessage publishes results; no consumer for video.results",
}

type mqSite struct {
	exchange, key, queue string
	pos                  string
}

type mqScan struct {
	publishes  []mqSite
	bindings   []mqSite
	consumers  map[string][]string // queue -> positions
	unresolved []string            // publish site không phân giải được
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("không xác định được đường dẫn file test")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..")
}

func parseSources(t *testing.T, root string) (*token.FileSet, []*ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	var files []*ast.File
	for _, dir := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			f, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}
			files = append(files, f)
			return nil
		})
		if err != nil {
			t.Fatalf("quét %s: %v", dir, err)
		}
	}
	if len(files) < 50 {
		t.Fatalf("chỉ quét được %d file .go — sai thư mục gốc %s?", len(files), root)
	}
	return fset, files
}

// stringConsts: hằng chuỗi cấp package theo tên; tên trùng mà khác giá trị thì không phân giải.
func stringConsts(files []*ast.File) map[string]string {
	out, ambiguous := map[string]string{}, map[string]bool{}
	for _, f := range files {
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}
			for _, spec := range gd.Specs {
				vs := spec.(*ast.ValueSpec)
				for i, name := range vs.Names {
					if i >= len(vs.Values) {
						continue
					}
					lit, ok := vs.Values[i].(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						continue
					}
					v, err := strconv.Unquote(lit.Value)
					if err != nil {
						continue
					}
					if prev, seen := out[name.Name]; seen && prev != v {
						ambiguous[name.Name] = true
					}
					out[name.Name] = v
				}
			}
		}
	}
	for n := range ambiguous {
		delete(out, n)
	}
	return out
}

func scanRabbitMQ(t *testing.T) mqScan {
	t.Helper()
	fset, files := parseSources(t, repoRoot(t))
	consts := stringConsts(files)
	resolve := func(e ast.Expr) (string, bool) {
		switch x := e.(type) {
		case *ast.BasicLit:
			if x.Kind == token.STRING {
				v, err := strconv.Unquote(x.Value)
				return v, err == nil
			}
		case *ast.Ident:
			v, ok := consts[x.Name]
			return v, ok
		case *ast.SelectorExpr:
			v, ok := consts[x.Sel.Name]
			return v, ok
		}
		return "", false
	}
	s := mqScan{consumers: map[string][]string{}}
	for _, f := range files {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			params := map[string]bool{}
			for _, fl := range fn.Type.Params.List {
				for _, n := range fl.Names {
					params[n.Name] = true
				}
			}
			isParam := func(e ast.Expr) bool {
				id, ok := e.(*ast.Ident)
				return ok && params[id.Name]
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				pos := func() string {
					p := fset.Position(n.Pos())
					return filepath.Base(p.Filename) + ":" + strconv.Itoa(p.Line)
				}
				if cl, ok := n.(*ast.CompositeLit); ok {
					name := ""
					switch ty := cl.Type.(type) {
					case *ast.Ident:
						name = ty.Name
					case *ast.SelectorExpr:
						name = ty.Sel.Name
					}
					if name == "WorkerConfig" {
						for _, el := range cl.Elts {
							if kv, ok := el.(*ast.KeyValueExpr); ok {
								if k, ok := kv.Key.(*ast.Ident); ok && k.Name == "QueueName" {
									if q, ok := resolve(kv.Value); ok {
										s.consumers[q] = append(s.consumers[q], pos())
									}
								}
							}
						}
					}
					return true
				}
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				a := call.Args
				switch sel.Sel.Name {
				case "PublishMessage", "PublishWithContext":
					if (sel.Sel.Name == "PublishMessage" && len(a) != 4) || (sel.Sel.Name == "PublishWithContext" && len(a) < 5) {
						return true
					}
					if isParam(a[1]) && isParam(a[2]) {
						return true // hàm bọc chỉ chuyển tiếp exchange/key của caller
					}
					ex, ok1 := resolve(a[1])
					key, ok2 := resolve(a[2])
					if !ok1 || !ok2 {
						s.unresolved = append(s.unresolved, pos())
						return true
					}
					s.publishes = append(s.publishes, mqSite{exchange: ex, key: key, pos: pos()})
				case "SetupQueue":
					if len(a) == 3 {
						ex, ok1 := resolve(a[0])
						q, ok2 := resolve(a[1])
						key, ok3 := resolve(a[2])
						if ok1 && ok2 && ok3 {
							s.bindings = append(s.bindings, mqSite{exchange: ex, queue: q, key: key, pos: pos()})
						}
					}
				case "QueueBind":
					if len(a) == 5 {
						q, ok1 := resolve(a[0])
						key, ok2 := resolve(a[1])
						ex, ok3 := resolve(a[2])
						if ok1 && ok2 && ok3 {
							s.bindings = append(s.bindings, mqSite{exchange: ex, queue: q, key: key, pos: pos()})
						}
					}
				case "ConsumeMessages":
					if len(a) == 3 {
						if q, ok := resolve(a[1]); ok {
							s.consumers[q] = append(s.consumers[q], pos())
						}
					}
				case "Consume":
					if len(a) == 7 {
						if q, ok := resolve(a[0]); ok {
							s.consumers[q] = append(s.consumers[q], pos())
						}
					}
				}
				return true
			})
		}
	}
	return s
}

// publishedQueues: queue -> các vị trí publish đi tới nó; route không tới queue nào gom vào unrouted.
func (s mqScan) publishedQueues() (map[string][]string, []string) {
	out := map[string][]string{}
	var unrouted []string
	for _, p := range s.publishes {
		routed := false
		for _, b := range s.bindings {
			if b.exchange == p.exchange && (b.key == p.key || b.key == "#") {
				out[b.queue] = append(out[b.queue], p.pos)
				routed = true
			}
		}
		if !routed {
			unrouted = append(unrouted, p.exchange+"/"+p.key+" @ "+p.pos)
		}
	}
	return out, unrouted
}

func TestQueueConsumerCoverage_EveryPublishedQueueHasAConsumer(t *testing.T) {
	s := scanRabbitMQ(t)
	if len(s.unresolved) > 0 {
		t.Errorf("publish không phân giải được exchange/routing key (gate sẽ mù): %v — dùng literal hoặc hằng chuỗi", s.unresolved)
	}
	published, unrouted := s.publishedQueues()
	if len(unrouted) > 0 {
		t.Errorf("publish tới exchange/key không bind vào queue nào (tin nhắn bị broker bỏ): %v", unrouted)
	}

	// Kiểm chứng bộ quét: các hàng đang chạy thật phải được nhận diện đủ hai chiều.
	for _, q := range []string{InvitationSendQueue, InvitationEventQueue, VideoProcessingQueue} {
		if len(published[q]) == 0 || len(s.consumers[q]) == 0 {
			t.Fatalf("bộ quét không thấy publisher/consumer của %q (publish=%v consume=%v) — bộ quét hỏng, gate vô nghĩa", q, published[q], s.consumers[q])
		}
	}

	var missing []string
	for q, at := range published {
		if len(s.consumers[q]) == 0 {
			if _, known := knownUnconsumedQueues[q]; !known {
				missing = append(missing, q+" (published at "+strings.Join(at, ", ")+")")
			}
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("hàng có publisher nhưng KHÔNG có consumer — tin nhắn sẽ kẹt mãi: %v. Thêm consumer hoặc bỏ publisher; "+
			"không thêm vào knownUnconsumedQueues.", missing)
	}
}

func TestQueueConsumerCoverage_KnownGapsStillAccurate(t *testing.T) {
	s := scanRabbitMQ(t)
	published, _ := s.publishedQueues()
	for q, why := range knownUnconsumedQueues {
		if len(published[q]) == 0 {
			t.Errorf("%q không còn publisher nào (%s) — lỗi đã hết: XOÁ mục này khỏi knownUnconsumedQueues", q, why)
		}
		if len(s.consumers[q]) > 0 {
			t.Errorf("%q nay đã có consumer tại %v — lỗi đã sửa: XOÁ mục này khỏi knownUnconsumedQueues", q, s.consumers[q])
		}
	}
}
