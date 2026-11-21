import { createHash, createHmac, randomBytes } from "node:crypto";
const DEFAULT_SCOPES = ["xtask.p2sp", "xindex.query"];
const sha256 = value => createHash("sha256").update(value).digest("hex");
export function signRequest({method, pathname, body="", productId, productSecret, now=Math.floor(Date.now()/1000), nonce=randomBytes(18).toString("base64url")}) {
  const bodyHash = sha256(body), timestamp = String(now);
  const canonical = ["THUNDER-AUTH-V1",method.toUpperCase(),pathname,timestamp,nonce,bodyHash].join("\n");
  return {"X-Product-ID":productId,"X-Timestamp":timestamp,"X-Nonce":nonce,"X-Content-SHA256":bodyHash,"X-Signature":createHmac("sha256",productSecret).update(canonical,"utf8").digest("hex")};
}
export class ThunderAuthClient {
  constructor(config, deviceId, fetchImpl=fetch) { this.config=config; this.deviceId=deviceId; this.fetch=fetchImpl; }
  async request(path, method="POST", payload={}) {
    const body = method==="GET" ? "" : JSON.stringify(payload);
    const headers = signRequest({method,pathname:path,body,productId:this.config.productId,productSecret:this.config.productSecret});
    if(method!=="GET") headers["Content-Type"]="application/json";
    const response = await this.fetch(this.config.authBaseUrl.replace(/\/+$/,"")+path,{method,headers,body:method==="GET"?undefined:body});
    const result = await response.json().catch(()=>({}));
    if(!response.ok || result.code!==0) throw new Error(result.message || ("thunder_auth_http_"+response.status));
    return result.data;
  }
  async getLoginToken() {
    const common={device_id:this.deviceId,product_version:this.config.appVersion||"0.1.0",sdk_platform:"linux-x64"};
    await this.request("/api/v1/auth/register","POST",common);
    const result=await this.request("/api/v1/auth/token","POST",common);
    if(!result.token || !Number.isSafeInteger(result.expires_at)) throw new Error("invalid_thunder_auth_token_response");
    return {token:result.token,tokenVersion:result.token_version,expiresAt:result.expires_at,scopes:result.scopes||DEFAULT_SCOPES};
  }
  async getStatus(){return this.request("/api/v1/auth/status","GET");}
}
