<template>
    <div class="bg-f2">
        <div class="bg-white bg-padding pb-20">
            <div class="mb-20 df jc-b ai-c list-toolbar">
                <el-button type="primary" icon="Plus" @click="addSite">添加站点</el-button>
            </div>
            <el-table :data="data" class="table-header">
                <el-table-column prop="host" label="域名" />
                <el-table-column label="操作">
                    <template #default="scope">
                        <el-button type="text" @click="openClearCache(scope.row)">
                            清空缓存
                        </el-button>
                        <el-button type="text" @click="$router.push(scope.row.detailUrl)">
                            修改
                        </el-button>
                        <el-button type="text" @click="toHttpsConfig(scope.row)">https配置</el-button>
                        <el-popconfirm
                            title="确定删除吗？"
                            placement="left-start"
                            popper-class="delete-popconfirm"
                            icon="WarningFilled"
                            confirm-button-type="danger"
                            @confirm="deleteRecord(scope.row.host, scope.row.storage_source.path_prefix, scope.row.ingressName)"
                        >
                            <template #reference>
                                <el-button type="text">删除</el-button>
                            </template>
                        </el-popconfirm>
                    </template>
                </el-table-column>
            </el-table>
        </div>

        <AddSite
            v-model="addSiteDialog.show"
            :data="addSiteDialog.form"
            @confirm="submitAddSite"
        />

        <el-dialog v-model="clearCache.show" title="清空缓存" width="500px">
            <el-form>
                <el-form-item label="路径">
                    <el-input
                        v-model="clearCache.path"
                        placeholder="请输入路径"
                    />
                </el-form-item>
            </el-form>
            <template #footer>
                <div class="dialog-footer">
                    <el-button @click="clearCache.show = false">取消</el-button>
                    <el-button type="primary" @click="clearCacheSubmit">确定</el-button>
                </div>
            </template>
        </el-dialog>
    </div>
</template>

<script>
import AddSite from "./add-site.vue";
import myAxios from "../../utils/index";

const createAddSiteForm = () => ({
    domains: [
        {
            url_after: "",
            auto_https: true,
        },
    ],
});

export default {
    name: "k8s-catch-list",
    components: {
        AddSite,
    },
    data() {
        return {
            data: [],
            addSiteDialog: {
                show: false,
                form: createAddSiteForm(),
            },
            clearCache: {
                show: false,
                group: "",
                path: "",
            },
        };
    },
    created() {
        this.getList();
    },
    methods: {
        toHttpsConfig(row){
            let data = { domainName: row.ingressName };
            window.$wujie?.bus?.$emit?.("domainCert",data);
            console.log('domainCert',data)
        },
        addSite() {
            this.addSiteDialog = {
                show: true,
                form: createAddSiteForm(),
            };
        },
        async submitAddSite(formData) {
            const backend = {
                service: {
                    name: window.$wujie?.props?.group,
                    port: { number: Number(window.$wujie?.props?.servicePort) }
                    // 测试
                    // name: 'w7-cdncache-qpiuzwnx',
                    // port: { number: 8000 },
                }
            }
            const domainToname = function (str) {
                return str.replace(/\*/g, 'x').replace(/(\.|\/|_)/g, '-').toLowerCase();
            }

            const createName = function (len) {
                len = len || 8;
                let s = 'abcdefghijklmnopqrstuvwxyz';
                let p = '';
                for (var i = 0; i < len; i++) {
                    p = p + s[parseInt(Math.random() * s.length)]
                }
                return p;
            }

            const domains = (formData?.domains || [])
                .map(item => ({
                    ...item,
                    url_after: item?.url_after?.trim()?.replace(/^https?:\/\//, '')?.replace(/\/+$/, ''),
                }))
                .filter(item => item.url_after);

            if (domains.length === 0) {
                this.$message.warning('请先输入至少一个域名');
                return;
            }

            const paneltoken = window.$wujie?.props?.paneltoken;
            const uniqueDomains = Array.from(new Map(domains.map(item => [item.url_after, item])).values());

            try {

                let item = uniqueDomains[0];
                const domain = item.url_after;
                const groupDomain = uniqueDomains.map(item => item.url_after).join(',');
                const children = uniqueDomains.slice(1).map(item => ({
                    name: 'ing-' + createName(),
                    host: item.url_after,
                    autoSsl: item.auto_https,
                    sslRedirect: false,
                }));

                let ingressName = 'ing-' + createName();
                let data = {
                    apiVersion: 'networking.k8s.io/v1',
                    kind: 'Ingress',
                    metadata: {
                        name: ingressName,
                        namespace: 'default',
                        annotations: {
                            'kubernetes.io/ingress.class': 'higress',
                            'higress.io/resource-definer': 'higress',
                            ...(children.length > 0 ? { 'w7.cc/child-hosts': JSON.stringify(children) } : {}),
                        },
                        labels: {
                            'higress.io/resource-definer': 'higress',
                            app: window.$wujie?.props?.group,
                            group: window.$wujie?.props?.group,
                            // 测试
                            // app: 'w7-cdncache-qpiuzwnx',
                            // group: 'w7-cdncache-qpiuzwnx',
                        },
                    },
                    spec: {
                        rules: [
                            {
                                host: domain,
                                http: {
                                    paths: [
                                        {
                                            path: '/',
                                            pathType: 'Prefix',
                                            backend: backend,
                                        },
                                    ],
                                },
                            },
                        ],
                    },
                }
                if (item.auto_https) {
                    data.metadata.annotations['cert-manager.io/cluster-issuer'] = 'w7-letsencrypt-prod';
                    data.metadata.annotations['cert-manager.io/renew-before'] = '30m';
                    data.metadata.annotations['w7.cc/ssl-redirect'] = 'false';
                    data.spec.tls = [{
                        hosts: [domain],
                        secretName: domainToname(domain) + '-tls-secret'
                    }]
                }
                await myAxios.post('/k8s-proxy/apis/networking.k8s.io/v1/namespaces/default/ingresses', data, {
                    baseURL: '',
                    // customToken: paneltoken,
                });

                await myAxios.post('/api/setting/set', {
                    group: groupDomain,
                    storage_source: {
                        endpoint: '',
                    },
                    minio: {
                        "access_key": "",
                        "secret_key": "",
                        "bucket": "",
                        "endpoint": "",
                        "region": ""
                    },
                    path_cache_rules: [],
                    path_key_cache_rules: [],
                    extra: {
                        ingress_name: ingressName,
                        storage_config: {
                            mode: "global",
                        },
                    },
                });

                this.addSiteDialog.show = false;
                this.getList();
                this.$message.success('创建成功');
            } catch (e) {}

        },
        openClearCache(row) {
            this.clearCache = {
                group: row.host,
                path: "",
                show: true,
            };
        },
        clearCacheSubmit() {
            myAxios
                .post("/api/util/clear-file-cache", {
                    group: this.clearCache.group,
                    path: this.clearCache.path,
                })
                .then(() => {
                    this.getList();
                    this.$message.success("操作成功");
                    this.clearCache.show = false;
                })
                .catch(() => {});
        },
        getList() {
            myAxios.post("/api/setting/list").then((res) => {
                let data = res.data.data;
                if (data) {
                    this.data = Object.entries(data)
                        .filter(([group]) => group !== "global")
                        .map((arr) => {
                            arr[1].host = arr[0];
                            let str = Object.entries(arr[1].storage_source || {})
                                .map(([k, v]) => `${encodeURIComponent(k)}=${encodeURIComponent(v)}`)
                                .join("&");
                            let ingressName = arr[1]?.extra?.ingress_name || "";
                            arr[1].ingressName = ingressName;
                            arr[1].detailUrl = `/cache/${arr[0]}?ingress_name=${ingressName}&${str}`;
                            return arr[1];
                        });
                }
            });
        },
        deleteRecord(group, pathPrefix,ingressName) {
            myAxios.post("/api/setting/del", { group, path_prefix: pathPrefix }).then(() => {
                
                return myAxios.delete(`/k8s-proxy/apis/networking.k8s.io/v1/namespaces/default/ingresses/${ingressName}`, {
                    baseURL: '',
                }).then(() => {
                    this.getList();
                    this.$message.success("操作成功");
                })
            });
        },
    },
};
</script>

<style>
.delete-popconfirm.el-popper{padding:10px; width:180px;}
.delete-popconfirm .el-popconfirm__action {
    display: flex;
    flex-direction: row-reverse;
    justify-content: flex-start;
    gap: 12px;
}

.delete-popconfirm .el-popconfirm__action .el-button + .el-button {
    margin-left: 0;
}
</style>
