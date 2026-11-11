const API="/api/v1";
const $=(id)=>document.getElementById(id);
const money=(value,currency="INR")=>{const n=Number(value);if(!Number.isFinite(n))return "—";return new Intl.NumberFormat("en-IN",{style:"currency",currency,maximumFractionDigits:2}).format(n)};
const number=(value)=>new Intl.NumberFormat("en-IN",{maximumFractionDigits:0}).format(Number(value)||0);
const pct=(value)=>{if(value===null||value===undefined||!Number.isFinite(Number(value)))return "—";const n=Number(value);return (n>0?"+":"")+n.toFixed(2)+"%";};
const esc=(s)=>String(s??"").replace(/[&<>"']/g,c=>({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[c]));
let companies=[];
let toastTimer;
function toast(message){const el=$("toast");el.textContent=message;el.classList.add("show");clearTimeout(toastTimer);toastTimer=setTimeout(()=>el.classList.remove("show"),2600);}
async function api(path,options={}){const response=await fetch(API+path,{...options,headers:{"Content-Type":"application/json",...(options.headers||{})}});const text=await response.text();let data={};try{data=text?JSON.parse(text):{};}catch{throw new Error("Server returned an unreadable response");}if(!response.ok)throw new Error(data.error||"Request failed ("+response.status+")");return data;}
function renderIndices(items){const root=$("indexGrid");if(!items?.length){root.innerHTML='<div class="empty">No stored index records yet.</div>';return;}root.innerHTML=items.map(x=>{const change=x.daily_change_pct;const cls=change>0?"positive":change<0?"negative":"neutral";return '<article class="index-card"><div class="index-name">'+esc(x.name)+'</div><div class="index-symbol">'+esc(x.symbol)+' · '+esc(x.exchange)+'</div><div class="index-price">'+money(x.close,x.currency||"INR")+'</div><div class="change '+cls+'">'+pct(change)+' <span class="muted">· '+esc(x.trading_date||"")+'</span></div></article>';}).join("");}
function renderCompanies(items){const root=$("companyRows");if(!items?.length){root.innerHTML='<tr><td colspan="6" class="empty">No companies found. Add companies through the API first.</td></tr>';return;}root.innerHTML=items.map(x=>{const cls=x.daily_change_pct>0?"positive":x.daily_change_pct<0?"negative":"neutral";return '<tr><td class="company-name">'+esc(x.name)+'</td><td class="symbol">'+esc(x.symbol)+'</td><td>'+esc(x.exchange)+'</td><td>'+money(x.close,x.currency||"INR")+'</td><td class="'+cls+'">'+pct(x.daily_change_pct)+'</td><td>'+esc(x.trading_date||"—")+'</td></tr>';}).join("");}
function filterCompanies(){const q=$("companySearch").value.trim().toLowerCase();renderCompanies(companies.filter(x=>(x.name+" "+x.symbol).toLowerCase().includes(q)));}
async function loadHealth(){try{await api("/health");$("apiStatus").textContent="Online";$("apiStatus").style.color="var(--teal)";$("apiStatusFoot").textContent="Backend responding";}catch{$("apiStatus").textContent="Offline";$("apiStatus").style.color="var(--red)";$("apiStatusFoot").textContent="Check API server";}}
async function loadOverview(){const data=await api("/market/overview");renderIndices(data.market_indices||[]);companies=data.companies||[];renderCompanies(companies);$("companyCount").textContent=number(companies.length);$("indexCount").textContent=number((data.market_indices||[]).length);$("lastUpdated").textContent="Updated "+new Date().toLocaleTimeString();return data;}
async function loadQuality(){const data=await api("/market/data-quality");const rows=data.data||[];$("recordCount").textContent=number(rows.reduce((sum,x)=>sum+Number(x.records||0),0));$("qualityRows").innerHTML=rows.length?rows.map(x=>'<tr><td><span class="company-name">'+esc(x.name)+'</span><div class="candidate-meta">'+esc(x.type)+' · '+esc(x.symbol)+'</div></td><td>'+number(x.records)+'</td><td>'+esc(x.first_date||"—")+'</td><td>'+esc(x.latest_date||"—")+'</td><td>'+number(x.large_move_flags)+'</td><td>'+number(x.long_calendar_gaps)+'</td><td>'+number(x.multiple_provider_dates)+'</td></tr>').join(""):'<tr><td colspan="7" class="empty">No data-quality rows available.</td></tr>';}
function renderScan(data){const u=data.universe||{};const summary=$("scanSummary");summary.classList.remove("hidden");summary.innerHTML='<span class="summary-chip">Scanned <b>'+number(u.companies_scanned)+'</b></span><span class="summary-chip">Eligible <b>'+number(u.eligible_candidates)+'</b></span><span class="summary-chip">Skipped <b>'+number(u.skipped_or_insufficient_data)+'</b></span>';const candidates=data.candidates||[];const root=$("scanResults");let html=candidates.map(item=>{const p=item.plan||{};return '<article class="candidate"><div class="candidate-head"><div><div class="candidate-title">'+esc(item.company_name)+' <span class="symbol">'+esc(item.symbol)+'</span></div><div class="candidate-meta">'+esc(item.exchange)+' · Last stored session '+esc(p.as_of_date)+'</div></div><div class="candidate-score">Research score '+Number(p.research_score||0).toFixed(1)+'</div></div><div class="metric-grid"><div class="metric"><span>RSI (14)</span><b>'+Number(p.rsi_14||0).toFixed(1)+'</b></div><div class="metric"><span>EMA 5 / 20</span><b>'+Number(p.ema_5||0).toFixed(2)+' / '+Number(p.ema_20||0).toFixed(2)+'</b></div><div class="metric"><span>ATR (14)</span><b>'+money(p.atr_14,item.currency)+'</b></div><div class="metric"><span>Avg volume (20)</span><b>'+number(p.average_volume_20)+'</b></div></div><div class="reasons">Historical simulation metrics only · Last close '+money(p.last_close,item.currency)+' · Hypothetical quantity '+number(p.simulated_quantity)+' · Not a trade instruction</div></article>';}).join("");const reasons=u.skip_reasons||{};const reasonEntries=Object.entries(reasons).sort((a,b)=>b[1]-a[1]);if(reasonEntries.length){html+='<div class="candidate"><div class="candidate-title">Why instruments were excluded</div><div class="reasons">'+reasonEntries.map(([reason,count])=>'<div class="reason-row"><span>'+esc(reason)+'</span><b>'+number(count)+'</b></div>').join("")+'</div></div>';}if(!candidates.length&&!reasonEntries.length)html+='<div class="empty">No eligible historical records. Check stored EOD coverage and provider selection.</div>';if(!candidates.length&&reasonEntries.length)html+='<div class="empty">No instruments passed this historical research screen. This is a valid result; the filter has not been loosened.</div>';root.innerHTML=html;}
async function runScan(event){event.preventDefault();const button=$("scanButton");button.disabled=true;button.textContent="Scanning…";$("scanResults").innerHTML='<div class="empty">Evaluating stored historical observations…</div>';try{const data=await api("/trade-planner/scan",{method:"POST",body:JSON.stringify({budget:Number($("budget").value),style:"swing",risk_percent:Number($("riskPercent").value),max_candidates:10,provider:"tejhq",exchange:$("exchange").value})});renderScan(data);}catch(err){$("scanResults").innerHTML='<div class="empty">'+esc(err.message)+'</div>';toast(err.message);}finally{button.disabled=false;button.innerHTML='Run historical scan <span>→</span>';}}
async function refreshAll(){try{await Promise.all([loadHealth(),loadOverview(),loadQuality()]);}catch(err){toast(err.message);}}
$("companySearch").addEventListener("input",filterCompanies);
$("scanForm").addEventListener("submit",runScan);
$("refreshBtn").addEventListener("click",refreshAll);
$("qualityBtn").addEventListener("click",()=>loadQuality().then(()=>toast("Data quality refreshed")).catch(e=>toast(e.message)));
document.querySelectorAll(".nav-link").forEach(link=>link.addEventListener("click",()=>{document.querySelectorAll(".nav-link").forEach(x=>x.classList.remove("active"));link.classList.add("active");}));
if("serviceWorker" in navigator){window.addEventListener("load",()=>navigator.serviceWorker.register("/app/sw.js").catch(()=>{}));}
refreshAll();


const toolDefinitions=[
 {id:"overview",label:"Market overview",method:"GET",path:()=>"/market/overview"},
 {id:"quality",label:"Data quality report",method:"GET",path:()=>"/market/data-quality"},
 {id:"company-list",label:"List companies",method:"GET",path:()=>"/companies"},
 {id:"company-create",label:"Create company",method:"POST",path:()=>"/companies",body:{name:"Example Company",symbol:"EXAMPLE",exchange:"NSE",country:"India",sector:"",industry:"",currency:"INR",website:""}},
 {id:"company-get",label:"Get company",method:"GET",path:v=>"/companies/"+v.company},
 {id:"company-update",label:"Update company",method:"PUT",path:v=>"/companies/"+v.company,body:{name:"Example Company",symbol:"EXAMPLE",exchange:"NSE",country:"India",sector:"",industry:"",currency:"INR",website:""}},
 {id:"company-delete",label:"Delete company",method:"DELETE",path:v=>"/companies/"+v.company,confirm:"Delete this company and its linked records? This cannot be undone."},
 {id:"news-collect",label:"Collect recent news",method:"POST",path:v=>"/news/collect?company_id="+v.company+"&query="+encodeURIComponent(v.query||"")},
 {id:"news-list",label:"List company news",method:"GET",path:v=>"/companies/"+v.company+"/news"},
 {id:"news-relevance",label:"Analyze news relevance",method:"POST",path:v=>"/companies/"+v.company+"/news/analyze"},
 {id:"news-ai",label:"AI analyze one news item",method:"POST",path:v=>"/companies/"+v.company+"/news/"+v.news+"/analyze-ai"},
 {id:"news-ai-batch",label:"AI analyze news batch",method:"POST",path:v=>"/companies/"+v.company+"/analyze-ai-batch"},
 {id:"news-analysis",label:"Get one AI news analysis",method:"GET",path:v=>"/companies/"+v.company+"/news/"+v.news+"/analysis-ai"},
 {id:"news-analyses",label:"List AI news analyses",method:"GET",path:v=>"/companies/"+v.company+"/news/analyses-ai"},
 {id:"intelligence",label:"Company intelligence summary",method:"GET",path:v=>"/companies/"+v.company+"/intelligence"},
 {id:"sentiment",label:"Sentiment history",method:"GET",path:v=>"/companies/"+v.company+"/sentiment-history?days="+v.days},
 {id:"prices-sync",label:"Sync company historical prices",method:"POST",path:v=>"/companies/"+v.company+"/prices/sync?from="+v.from+"&to="+v.to,confirm:"Fetch and store historical end-of-day prices for this company?"},
 {id:"prices-list",label:"Get company price history",method:"GET",path:v=>"/companies/"+v.company+"/prices?from="+v.from+"&to="+v.to+"&limit=500"},
 {id:"company-analytics",label:"Company market analytics",method:"GET",path:v=>"/companies/"+v.company+"/market-analytics?days="+v.days},
 {id:"news-correlation",label:"News and price correlation",method:"GET",path:v=>"/companies/"+v.company+"/news-price-correlation?limit="+v.days},
 {id:"backtest",label:"Historical moving-average backtest",method:"POST",path:v=>"/companies/"+v.company+"/backtest",body:{from:"2025-01-01",to:"2026-11-11",fast_period:5,slow_period:20,initial_capital:100000,commission_bps:10,slippage_bps:5,annual_risk_free_rate:0,provider:"tejhq"}},
 {id:"forecast",label:"Experimental model forecast",method:"POST",path:v=>"/companies/"+v.company+"/forecast",body:{horizon_sessions:5,provider:"tejhq",ridge_lambda:1}},
 {id:"forecast-eval",label:"Walk-forward forecast evaluation",method:"POST",path:v=>"/companies/"+v.company+"/forecast/evaluate",body:{provider:"tejhq",horizons:[1,5,10,20],initial_train_size:120,minimum_train_size:60,test_size:20,step_size:20,ridge_lambda:1}},
 {id:"paper-scan",label:"Budget-based paper research scan",method:"POST",path:()=>"/trade-planner/scan",body:{budget:10000,style:"swing",risk_percent:1,max_candidates:10,provider:"tejhq",exchange:"NSE"}},
 {id:"event-study",label:"Historical news event study",method:"GET",path:v=>"/companies/"+v.company+"/event-study?index_id="+v.index+"&from="+v.from+"&to="+v.to+"&pre=5&post=5&provider="+encodeURIComponent(v.provider)+"&index_provider=yahoo"},
 {id:"index-sync",label:"Sync index historical prices",method:"POST",path:v=>"/indices/"+v.index+"/prices/sync?from="+v.from+"&to="+v.to,confirm:"Fetch and store historical index data?"},
 {id:"index-prices",label:"Get index price history",method:"GET",path:v=>"/indices/"+v.index+"/prices?from="+v.from+"&to="+v.to+"&limit=500"},
 {id:"index-analytics",label:"Index analytics",method:"GET",path:v=>"/indices/"+v.index+"/analytics?days="+Math.max(2,v.days)},
 {id:"indices-compare",label:"Compare two market indices",method:"GET",path:v=>"/indices/compare?left_id="+v.index+"&right_id="+v.rightIndex+"&days="+Math.max(2,v.days)},
 {id:"company-index",label:"Compare company with index",method:"GET",path:v=>"/companies/"+v.company+"/compare/indices/"+v.index+"?days="+Math.max(2,v.days)}
];
function selectedTool(){return toolDefinitions.find(x=>x.id===$("toolOperation").value)||toolDefinitions[0];}
function toolValues(){return {company:Math.max(1,Number($("toolCompanyId").value)||1),index:Math.max(1,Number($("toolIndexId").value)||1),rightIndex:Math.max(1,Number($("toolRightIndexId").value)||2),news:Math.max(1,Number($("toolNewsId").value)||1),from:$("toolFrom").value,to:$("toolTo").value,days:Math.max(1,Number($("toolDays").value)||30),provider:$("toolProvider").value.trim()||"tejhq",query:""};}
function setToolBody(){const item=selectedTool();$("toolBody").value=item.body?JSON.stringify(item.body,null,2):"{}";$("toolBody").disabled=!item.body&&item.method!=="POST"&&item.method!=="PUT";}
function setupWorkbench(){
 const select=$("toolOperation");if(!select)return;
 select.innerHTML=toolDefinitions.map(x=>'<option value="'+esc(x.id)+'">'+esc(x.label)+' · '+x.method+'</option>').join("");
 select.addEventListener("change",setToolBody);setToolBody();
 $("resetToolButton").addEventListener("click",()=>{setToolBody();$("toolResult").textContent="Parameters reset.";});
 $("runToolButton").addEventListener("click",runSelectedTool);
}
async function runSelectedTool(){
 const item=selectedTool(), values=toolValues(), button=$("runToolButton"), result=$("toolResult"), status=$("toolStatus");
 if(!values.from||!values.to){if(["prices-sync","prices-list","index-sync","index-prices","event-study"].includes(item.id)){toast("Select both date values first.");return;}}
 if(item.confirm&&!window.confirm(item.confirm))return;
 let body;
 if(item.body||["company-create","company-update","backtest","forecast","forecast-eval","paper-scan"].includes(item.id)){
  try{body=JSON.parse($("toolBody").value||"{}");}catch(e){toast("Request body JSON is invalid.");return;}
 }
 let path=item.path(values);
 if(item.id==="company-list"){const q=prompt("Optional company name or symbol search:","");if(q)path+="?search="+encodeURIComponent(q);}
 if(item.id==="news-collect"){const q=prompt("News search query (blank uses company name):","");if(q)path=path.replace(/query=$/,"query="+encodeURIComponent(q));}
 button.disabled=true;button.textContent="Running…";status.textContent=item.method+" "+path;result.textContent="Request in progress…";
 try{
  const data=await api(path,{method:item.method,...(body!==undefined?{body:JSON.stringify(body)}:{})});
  result.textContent=JSON.stringify(data,null,2);status.textContent="Success · "+item.method;toast("Operation completed");
  if(["overview","quality","company-create","company-update","company-delete","prices-sync","index-sync","news-collect"].includes(item.id))refreshAll().catch(()=>{});
 }catch(err){result.textContent=JSON.stringify({error:err.message},null,2);status.textContent="Request failed";toast(err.message);}
 finally{button.disabled=false;button.innerHTML='Run selected operation <span>→</span>';}
}
setupWorkbench();
