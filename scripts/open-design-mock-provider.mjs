// Local-only OpenAI-compatible upstream fixture for browser QA. No real AI key is used.
import http from "node:http";
let calls=0;
http.createServer(async(req,res)=>{
 if(req.method!=="POST"||req.url!=="/v1/chat/completions"){res.writeHead(404).end();return;}
 let raw="";for await(const chunk of req)raw+=chunk;const input=JSON.parse(raw);const request=JSON.parse(input.messages.at(-1).content);const call=++calls;
 console.log(JSON.stringify({call,output:request.output,count:request.count,documents:request.documents.length,refinement:!!request.current,skills:(request.skills??[]).map(s=>s.name),history:(request.history??[]).length,hasTokens:input.messages[0].content.includes("--primary:")}));
 if(request.prompt.includes("KIEM_THU_LOI")){res.writeHead(503,{"Content-Type":"application/json"}).end('{}');return;}
 if(request.prompt.includes("KIEM_THU_CHAM")){res.on("close",()=>console.log(JSON.stringify({call,cancelled:true})));return;}
 const artifact=request.output==="slides"?{kind:"slides",title:`Bài giảng Python · DEMO ${call}`,slides:Array.from({length:request.count},(_,i)=>({title:i===0?"Vòng lặp trong Python":i===request.count-1?"Tổng kết bài học":`Thực hành vòng lặp · ${i}`,subtitle:"DEMO kiểm thử local, chưa gọi Viettel AI thật",bullets:i===0?["Lớp 10 · Tin học"]:["Vòng lặp for duyệt từng phần tử của một tập hợp.","Dùng range() để tạo dãy số.","Thực hành: tính tổng các số từ 1 đến 10."],notes:"Nguồn: tài liệu đính kèm. Mời học sinh giải thích từng bước.",layout:i===0?"cover":i===request.count-1?"summary":"content"}))}:{kind:"quiz",title:`Trắc nghiệm Python · DEMO ${call}`,questions:Array.from({length:request.count},(_,i)=>({question:i===0?"Kết quả của 2 + 2 là gì?":`Câu ${i+1}: Lệnh nào tạo dãy số 0, 1, 2?`,options:i===0?["3","4","5","6"]:["range(3)","range(1,3)","range(2)","range(3,0)"],correct_answer:i===0?1:0,explanation:i===0?"Hai cộng hai bằng bốn, chọn B.":"range(3) bắt đầu từ 0 và dừng trước 3."}))};
 setTimeout(()=>{res.writeHead(200,{"Content-Type":"application/json"});res.end(JSON.stringify({choices:[{message:{role:"assistant",content:JSON.stringify(artifact)},finish_reason:"stop"}]}));},1200);
}).listen(15003,"127.0.0.1",()=>console.log("QA provider at 127.0.0.1:15003"));
