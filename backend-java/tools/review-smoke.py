"""Run real HTTP review flows against an isolated Java process and H2 database.
Usage: python tools/review-smoke.py [path/to/application.jar]
Set ERP_TEST_CLASSPATH only when validating directly compiled classes.
"""
import os,sys,json,time,subprocess,urllib.request,urllib.error,concurrent.futures,pathlib,tempfile
base='http://127.0.0.1:16085';checks=[];token=''
def call(method,path,body=None,t=None,headers=None):
 h={'Content-Type':'application/json'}
 if t is None:t=token
 if t:h['Authorization']='Bearer '+t
 if headers:h.update(headers)
 req=urllib.request.Request(base+urllib.parse.quote(path,safe='/?=&,'),data=json.dumps(body).encode() if body is not None else None,headers=h,method=method)
 try:
  with urllib.request.urlopen(req,timeout=20) as r:return r.status,json.load(r)
 except urllib.error.HTTPError as e:
  try:return e.code,json.load(e)
  except:return e.code,{}
def check(name,condition,detail=None):
 checks.append({'name':name,'passed':bool(condition),'detail':detail});print(('PASS ' if condition else 'FAIL ')+name,flush=True)
 if not condition and detail:print(str(detail)[:500],flush=True)
def api(method,path,body=None):
 status,result=call(method,'/api/v1'+path,body)
 if status!=200 or result.get('code')!=0:raise AssertionError((path,status,result))
 return result.get('data')
cp=os.getenv('ERP_TEST_CLASSPATH');cmd=['java','-cp',cp,'com.kasinlo.erp.ErpApplication'] if cp else ['java','-jar',sys.argv[1]]
cmd+=['--server.port=16085','--spring.datasource.url='+os.getenv('ERP_TEST_DATABASE_URL','jdbc:h2:mem:smoke;MODE=PostgreSQL;DATABASE_TO_LOWER=TRUE;DEFAULT_NULL_ORDERING=HIGH;DB_CLOSE_DELAY=-1'),'--erp.demo-data=true']
log=open('/tmp/kasinlo-java-test-server.log','w');process=subprocess.Popen(cmd,stdout=log,stderr=log)
try:
 for i in range(120):
  if process.poll() is not None:raise RuntimeError('Java exited; inspect /tmp/kasinlo-java-test-server.log')
  try:
   if call('GET','/health',t='')[0]==200:break
  except Exception:pass
  time.sleep(.25)
 else:raise RuntimeError('Java startup timed out')
 status,result=call('POST','/api/v1/auth/login',{'username':'admin','password':'admin123456'},t='');check('管理员登录',status==200,result);token=result['data']['accessToken']
 check('菜单动态路由',len(api('GET','/profile/routes'))>=2)
 check('未登录访问被拒绝',call('GET','/api/v1/inv/products',t='')[0]==401)
 for path in ['/profile','/system/users','/system/roles','/system/menus','/system/depts','/system/logs/operations','/system/logs/logins','/system/logs/modules','/inv/overview','/inv/customers','/inv/suppliers','/inv/categories','/inv/categories/options','/inv/products','/inv/products/pick','/inv/products/duplicates?name=螺栓','/inv/orders?bizType=purchase','/inv/orders?bizType=sales','/inv/contracts','/inv/stock-orders','/inv/inventory','/inv/finance/receivables','/inv/finance/payables','/inv/print-templates','/inv/report/summary','/inv/report/trend','/inv/report/ranking','/inv/report/category','/inv/report/inventory','/inv/report/stock-ranking','/inv/report/detail','/inv/report/contract','/inv/report/contract-ledger','/ops/check','/ops/backups','/ops/cleanup/preview','/ops/cleanup/scopes','/ops/upgrade','/ops/upgrade/records']:
  status,result=call('GET','/api/v1'+path);check('读取 '+path,status==200 and result.get('code')==0,result if status!=200 else None)
 product=api('POST','/inv/products',{'name':'迁移回归商品'})['id'];p=api('GET',f'/inv/products/{product}');spec=p['specs'][0];unit=spec['units'][0];item={'productId':product,'specId':spec['id'],'unitId':unit['id'],'qty':100,'price':2.35}
 customer=api('POST','/inv/customers',{'name':'迁移测试客户'})['id'];supplier=api('POST','/inv/suppliers',{'name':'迁移测试供应商'})['id']
 order={'bizType':'purchase','partyId':supplier,'orderDate':'2026-10-24','items':[item]};oid=api('POST','/inv/orders',order)['id'];api('PUT',f'/inv/orders/{oid}/status',{'status':2});check('采购审核入库',api('GET',f'/inv/products/{product}')['specs'][0]['stock']==100)
 check('重复审核拒绝',call('PUT',f'/api/v1/inv/orders/{oid}/status',{'status':2})[0]==409)
 api('PUT',f'/inv/orders/{oid}/reverse',{});check('采购反审核回滚库存',api('GET',f'/inv/products/{product}')['specs'][0]['stock']==0)
 api('PUT',f'/inv/orders/{oid}/status',{'status':2})
 sales=[]
 for i in range(2):sales.append(api('POST','/inv/orders',{'bizType':'sales','partyId':customer,'orderDate':'2026-10-24','items':[{**item,'qty':80}]})['id'])
 with concurrent.futures.ThreadPoolExecutor(2) as pool:results=list(pool.map(lambda sid:call('PUT',f'/api/v1/inv/orders/{sid}/status',{'status':2}),sales))
 check('并发销售审核只有一笔成功',sorted(r[0] for r in results)==[200,409],results)
 check('并发扣减不超卖',api('GET',f'/inv/products/{product}')['specs'][0]['stock']==20)
 failed=sales[next(i for i,r in enumerate(results) if r[0]!=200)];check('失败审核事务回滚',api('GET',f'/inv/orders/{failed}')['status']==1)
 passed=sales[next(i for i,r in enumerate(results) if r[0]==200)];amount=api('GET',f'/inv/orders/{passed}')['totalAmount'];check('金额按分计算',amount==188)
 body={'orderId':passed,'amount':50};status,r=call('POST','/api/v1/inv/finance/settlements',body,headers={'Idempotency-Key':'smoke-settle'});status2,r2=call('POST','/api/v1/inv/finance/settlements',body,headers={'Idempotency-Key':'smoke-settle'});check('重复收款幂等',status==200 and status2==200 and api('GET',f'/inv/orders/{passed}')['settledAmount']==50)
 check('超额收款拒绝',call('POST','/api/v1/inv/finance/settlements',{'orderId':passed,'amount':999})[0]==409)
 check('已收款单据反审核拒绝',call('PUT',f'/api/v1/inv/orders/{passed}/reverse',{})[0]==409)
 cid=api('POST','/inv/contracts',{'name':'回归合同','bizType':'sales','partyId':customer,'signDate':'2026-10-24','amount':1000,'warrantyMonths':12,'plans':[{'seq':1,'name':'预付款','rate':30,'kind':1,'triggerNode':'sign'},{'seq':2,'name':'验收款','rate':70,'kind':3,'triggerNode':'accept'}]})['id'];c=api('GET',f'/inv/contracts/{cid}');check('合同创建为待审核',c['status']==0)
 pid=c['plans'][0]['id'];check('待审核合同收款拒绝',call('POST','/api/v1/inv/contract-plans/settle',{'planId':pid,'amount':100})[0]==409)
 api('POST',f'/inv/contracts/{cid}/audit',{});api('POST','/inv/contract-plans/settle',{'planId':pid,'amount':100});api('POST',f'/inv/contracts/{cid}/accept',{'acceptedAt':'2026-10-24','acceptResult':'合格','warrantyMonths':12});c=api('GET',f'/inv/contracts/{cid}');check('合同分期及质保计算',c['settledAmount']==100 and c['warrantyEndAt']=='2027-10-24')
 d=api('POST','/inv/print-templates',{'name':'回归打印模板','code':'smoke-template','paper':'A4','content':'{"title":"评审打印"}'})['id'];check('打印模板持久化',any(r['id']==d for r in api('GET','/inv/print-templates')['list']))
 status,demo=call('POST','/api/v1/auth/login',{'username':'demo','password':'demo123456'},t='');check('只读账号拒绝写操作',call('POST','/api/v1/inv/customers',{'name':'不应创建'},t=demo['data']['accessToken'])[0]==403)
 check('接口契约清单',api('GET','/migration/coverage')['routes']==129)
except Exception as e:check('流程执行',False,repr(e));raise
finally:
 process.terminate();process.wait(timeout=10);log.close();path=pathlib.Path('/tmp/kasinlo-review-results.json');path.write_text(json.dumps({'passed':sum(x['passed'] for x in checks),'failed':sum(not x['passed'] for x in checks),'checks':checks},ensure_ascii=False,indent=2));print('RESULT',sum(x['passed'] for x in checks),'passed',sum(not x['passed'] for x in checks),'failed',flush=True)
if any(not x['passed'] for x in checks):sys.exit(1)
