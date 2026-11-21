let mod, sdk, initialized=false, loggedTokenVersion=null;
async function loadSdk(){if(!mod){try{mod=await import("@xunlei-open/dlsdk");}catch(e){throw new Error("thunder_sdk_native_module_unavailable: "+(e?.code||e?.message||"unknown"));}}return mod;}
export async function initializeSdk({appId,appVersion,configPath}){
  if(process.platform!=="linux"||process.arch!=="x64")throw new Error("thunder_sdk_unsupported_platform: official Node.js SDK currently documents Linux x64 only");
  const m=await loadSdk();if(!sdk)sdk=new m.XLDownloadAPI();
  const result=sdk.init({appId,appVersion,configPath,saveTasks:true});
  if(result!==m.ERROR_SUCCESS&&result!==m.ERROR_ALREADY_INIT)throw new Error("thunder_sdk_init_failed:"+result);
  initialized=true;return {initialized:true,result};
}
export async function loginSdk(token,version){
  if(!initialized||!sdk)throw new Error("thunder_sdk_not_initialized");
  if(loggedTokenVersion===version)return {loggedIn:true,reused:true};
  const m=await loadSdk(),result=sdk.login(token);
  if(!result||result.result!==m.ERROR_SUCCESS)throw new Error("thunder_sdk_login_failed:"+(result?.result??"unknown"));
  loggedTokenVersion=version;return {loggedIn:true,tokenVersion:version};
}
function getSdk(){if(!initialized||!sdk)throw new Error("thunder_sdk_not_initialized");return sdk;}
export function createP2spTask(input){const r=getSdk().createP2spTask(input);if(!r||r.result!==0)throw new Error("thunder_sdk_create_task_failed:"+(r?.result??"unknown"));return {taskId:String(r.taskId)};}
export function startTask(id){const r=getSdk().startTask(BigInt(id));if(r!==0)throw new Error("thunder_sdk_start_task_failed:"+r);}
export function stopTask(id){const r=getSdk().stopTask(BigInt(id));if(r!==0)throw new Error("thunder_sdk_stop_task_failed:"+r);}
export function deleteTask(id){const r=getSdk().deleteTask(BigInt(id),false);if(r!==0)throw new Error("thunder_sdk_delete_task_failed:"+r);}
export function getTaskState(id){const r=getSdk().getTaskState(BigInt(id));if(!r||r.result!==0)throw new Error("thunder_sdk_task_state_failed:"+(r?.result??"unknown"));return Object.fromEntries(Object.entries(r.state).map(([k,v])=>[k,typeof v==="bigint"?v.toString():v]));}
export function uninitializeSdk(){if(initialized&&sdk)sdk.uninit();initialized=false;sdk=undefined;loggedTokenVersion=null;}
