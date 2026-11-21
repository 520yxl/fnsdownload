import http from "node:http";
import {createHash,randomBytes,scryptSync,timingSafeEqual} from "node:crypto";
import {readFile,stat,mkdir} from "node:fs/promises";
import path from "node:path";
import {ensureStore,loadConfig,saveConfig,loadTasks,saveTasks,sdkConfigPath} from "./store.js";
import {ThunderAuthClient} from "./thunder-auth.js";
import * as sdk from "./thunder-sdk.js";
const PORT=Number(process.env.PORT||8088), PUBLIC=path.resolve("public"), sessions=new Map();
let sdkReady=false;
const makePasswordHash=s=>{const salt=randomBytes(16);const derived=scryptSync(s,salt,64);return salt.toString("hex")+":"+derived.toString("hex");};\nconst verifyPassword=(s,stored)=>{try{const [saltHex,hashHex]=stored.split(":");const expected=Buffer.from(hashHex,"hex"),actual=scryptSync(s,Buffer.from(saltHex,"hex"),64);return expected.length===actual.length&&timingSafeEqual(expected,actual);}catch{return false;}};\nfunction originAllowed(req){const origin=req.headers.origin;if(!origin)return true;try{return new URL(origin).host===req.headers.host;}catch{return false;}}
const json=(res,status,data)=>{res.writeHead(status,{"Content-Type":"application/json; charset=utf-8","Cache-Control":"no-store"});res.end(JSON.stringify(data));};
const cookies=req=>Object.fromEntries((req.headers.cookie||"").split(";").map(x=>x.trim().split("=")).filter(x=>x.length===2));
const setSession=(res,sid)=>res.setHeader("Set-Cookie","moxyi_session="+sid+"; Path=/; HttpOnly; SameSite=Strict; Max-Age=43200");
async function readBody(req){const chunks=[];let n=0;for await(const c of req){n+=c.length;if(n>1048576)throw Error("request_too_large");chunks.push(c);}return n?JSON.parse(Buffer.concat(chunks).toString("utf8")):{};}
function authed(req){const id=cookies(req).moxyi_session,expiry=sessions.get(id);if(!id||!expiry||expiry<Date.now()){sessions.delete(id);return false;}return true;}
async function connectSdk(c){if(!c.thunderAppId||!c.authBaseUrl||!c.productSecret)throw Error("complete_authorization_settings_first");await mkdir(c.downloadDir,{recursive:true});await sdk.initializeSdk({appId:c.thunderAppId,appVersion:c.appVersion,configPath:sdkConfigPath()});const auth=new ThunderAuthClient(c,c.deviceId);const token=await auth.getLoginToken();await sdk.loginSdk(token.token,token.tokenVersion);sdkReady=true;return {tokenVersion:token.tokenVersion,expiresAt:token.expiresAt,scopes:token.scopes};}
async function route(req,res,url){
 const p=url.pathname;
 if(p==="/api/health")return json(res,200,{ok:true,service:"fnsdownload",version:"0.1.0"});
 if(p==="/api/setup"&&req.method==="GET"){const c=await loadConfig();return json(res,200,{setupRequired:!c.adminPasswordHash});}
 if(p==="/api/setup"&&req.method==="POST"){
  const c=await loadConfig();if(c.adminPasswordHash)return json(res,409,{error:"already_configured"});
  const b=await readBody(req);if(typeof b.password!=="string"||b.password.length<12)return json(res,400,{error:"password_must_be_at_least_12_chars"});
  if(!/^https:\/\//.test(b.authBaseUrl||"")||!b.productSecret||!b.thunderAppId)return json(res,400,{error:"invalid_authorization_settings"});
  c.adminPasswordHash=makePasswordHash(b.password);c.authBaseUrl=b.authBaseUrl.replace(/\/+$/,"");c.productId=b.productId||"moxyi-download";c.productSecret=b.productSecret;c.thunderAppId=b.thunderAppId;c.downloadDir="/downloads";await saveConfig(c);
  const sid=randomBytes(32).toString("base64url");sessions.set(sid,Date.now()+43200000);setSession(res,sid);return json(res,201,{configured:true});
 }
 if(p==="/api/login"&&req.method==="POST"){const c=await loadConfig(),b=await readBody(req);if(!c.adminPasswordHash||!verifyPassword(String(b.password||""),c.adminPasswordHash))return json(res,401,{error:"invalid_credentials"});const sid=randomBytes(32).toString("base64url");sessions.set(sid,Date.now()+43200000);setSession(res,sid);return json(res,200,{loggedIn:true});}
 if(p==="/api/logout"&&req.method==="POST"){sessions.delete(cookies(req).moxyi_session);res.setHeader("Set-Cookie","moxyi_session=; Path=/; HttpOnly; SameSite=Strict; Max-Age=0");return json(res,200,{loggedOut:true});}
 if(!authed(req))return json(res,401,{error:"login_required"});
 if(p==="/api/config"&&req.method==="GET"){const c=await loadConfig();return json(res,200,{authBaseUrl:c.authBaseUrl,productId:c.productId,thunderAppId:c.thunderAppId,downloadDir:c.downloadDir,sdkReady});}
 if(p==="/api/config"&&req.method==="POST"){const c=await loadConfig(),b=await readBody(req);if(b.authBaseUrl&&!/^https:\/\//.test(b.authBaseUrl))return json(res,400,{error:"auth_url_must_be_https"});c.authBaseUrl=b.authBaseUrl??c.authBaseUrl;c.productId=b.productId??c.productId;c.productSecret=b.productSecret||c.productSecret;c.thunderAppId=b.thunderAppId??c.thunderAppId;await saveConfig(c);sdkReady=false;return json(res,200,{saved:true});}
 if(p==="/api/sdk/login"&&req.method==="POST"){try{return json(res,200,{loggedIn:true,...await connectSdk(await loadConfig())});}catch(e){sdkReady=false;return json(res,502,{error:e.message});}}
 if(p==="/api/tasks"&&req.method==="GET"){const tasks=await loadTasks();return json(res,200,tasks.map(t=>{try{return sdkReady?{...t,state:sdk.getTaskState(t.taskId)}:t;}catch{return t;}}));}
 if(p==="/api/tasks"&&req.method==="POST"){
  const b=await readBody(req);if(!sdkReady)return json(res,409,{error:"sdk_not_logged_in"});let u;try{u=new URL(b.url);}catch{return json(res,400,{error:"invalid_url"});}
  if(!["http:","https:"].includes(u.protocol))return json(res,400,{error:"only_http_https_supported"});
  const name=String(b.saveName||decodeURIComponent(u.pathname.split("/").pop()||"download.bin"));
  if(!name||name==="."||name===".."||path.basename(name)!==name||/[\\/]/.test(name))return json(res,400,{error:"invalid_save_name"});
  try{const c=await loadConfig(),created=sdk.createP2spTask({url:u.toString(),savePath:c.downloadDir,saveName:name});sdk.startTask(created.taskId);const tasks=await loadTasks();tasks.unshift({taskId:created.taskId,url:u.toString(),saveName:name,createdAt:Date.now()});await saveTasks(tasks);return json(res,201,{taskId:created.taskId});}catch(e){return json(res,502,{error:e.message});}
 }
 const action=p.match(/^\/api\/tasks\/([0-9]+)\/(pause|resume)$/);
 if(action&&req.method==="POST"){if(!sdkReady)return json(res,409,{error:"sdk_not_logged_in"});try{action[2]==="pause"?sdk.stopTask(action[1]):sdk.startTask(action[1]);return json(res,200,{ok:true});}catch(e){return json(res,502,{error:e.message});}}
 const del=p.match(/^\/api\/tasks\/([0-9]+)$/);
 if(del&&req.method==="DELETE"){if(!sdkReady)return json(res,409,{error:"sdk_not_logged_in"});try{sdk.deleteTask(del[1]);await saveTasks((await loadTasks()).filter(t=>t.taskId!==del[1]));return json(res,200,{deleted:true});}catch(e){return json(res,502,{error:e.message});}}
 return json(res,404,{error:"not_found"});
}
await ensureStore();
const server=http.createServer(async(req,res)=>{try{const url=new URL(req.url,"http://localhost");if(url.pathname.startsWith("/api/"))return await route(req,res,url);const name=url.pathname==="/ "?"index.html":decodeURIComponent(url.pathname==="/"?"index.html":url.pathname.slice(1));const file=path.resolve(PUBLIC,name);if(!file.startsWith(PUBLIC+path.sep))return json(res,403,{error:"forbidden"});const info=await stat(file);if(!info.isFile())return json(res,404,{error:"not_found"});const ext=path.extname(file),types={".html":"text/html; charset=utf-8",".js":"text/javascript; charset=utf-8",".css":"text/css; charset=utf-8"};res.writeHead(200,{"Content-Type":types[ext]||"application/octet-stream","X-Content-Type-Options":"nosniff","Referrer-Policy":"no-referrer"});res.end(await readFile(file));}catch(e){json(res,500,{error:e.message==="ENOENT"?"not_found":"internal_error"});}});
server.listen(PORT,"0.0.0.0",()=>console.log("MoxYi Download listening on :"+PORT));
process.on("SIGTERM",()=>{try{sdk.uninitializeSdk();}catch{}server.close(()=>process.exit(0));});
