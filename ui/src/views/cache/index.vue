<template>
    <div class="padding-20">
        <div class="mb-20">
            
            <div class="com-back df ai-c">
                <span class="backbtn df ai-c" @click="$router.go(-1)">
                    <el-icon class="backicon" color="rgb(var(--primary-6))" :size="20"><Back /></el-icon>
                    <span style="color: #86909c; font-size: 16px">站点配置</span>
                    <span style="color: #c9cdd4; padding: 0 5px; font-weight: 900; font-size: 16px">/</span>
                    <span style="font-size: 16px">详情</span>
                </span>
            </div>
        </div>
        <a-tabs v-model:active-key="tab" class="file-cache-tabs">
            <a-tab-pane key="2" title="缓存配置"></a-tab-pane>
            <a-tab-pane key="1" title="缓存镜像仓库"></a-tab-pane>
        </a-tabs>
        <div v-if="tab == '1'">
            <div class="repository-toolbar">
                <el-radio-group v-model="cacheRepository.mode">
                    <el-radio-button label="global">全局配置</el-radio-button>
                    <el-radio-button label="custom">自定义</el-radio-button>
                </el-radio-group>
            </div>

            <el-alert
                v-if="!globalRepositoryLoading && cacheRepository.mode === 'global' && !globalRepository.repository_url"
                title="尚未配置全局缓存仓库，请先完成全局配置"
                type="warning"
                show-icon
                :closable="false"
                class="mb-20"
            />

            <el-skeleton v-if="globalRepositoryLoading" :rows="2" animated />
            <CacheRepositoryForm
                v-else
                :model-value="activeCacheRepository"
                :repository-disabled="cacheRepository.mode === 'global'"
                :inherited="cacheRepository.mode === 'global'"
                @update:model-value="updateCacheRepository"
            />
        </div>
        <div v-if="tab == '2'">
            <div class="b mt-10">节点缓存过期配置</div>
            <div class="mt-4 fs-12 c-99">
                节点缓存过期配置可以设置源站资源在节点的缓存过期时间，以调整源站资源在节点缓存更新频率。您可以根据业务需求，按目录、文件后缀名、文件全路径配置资源的缓存过期时间。
            </div>
            <table class="com-table mt-10">
                <tbody>
                    <tr class="thead">
                        <td>是否缓存</td>
                        <td>
                            <div class="df ai-c">
                                <span>缓存时间(分钟)</span>
                                <a-tooltip content="0值为永不过期">
                                    <el-icon
                                        class="cursor c-99"
                                        style="margin-left: 4px"
                                        ><WarningFilled
                                    /></el-icon>
                                </a-tooltip>
                            </div>
                        </td>
                        <td>类型</td>
                        <td>路径</td>
                        <td>
                            <span class="df ai-c">
                                <span class="cursor">权重</span>
                                <a-tooltip content="权重值越小优先级越高">
                                    <el-icon
                                        class="cursor c-99"
                                        style="margin-left: 4px"
                                        ><WarningFilled
                                    /></el-icon>
                                </a-tooltip>
                            </span>
                        </td>
                        <td>流式返回</td>
                        <td>操作</td>
                    </tr>
                    <tr
                        v-for="(item, index) in path_cache_rules"
                        :key="index"
                        style="background: var(--color-neutral-1)"
                    >
                        <td>
                            <a-switch v-model="item.enable"></a-switch>
                        </td>
                        <td>
                            <a-input
                                v-if="item.enable"
                                v-model="item.cache_ttl"
                                type="number"
                                placeholder="缓存时间"
                                style="width: 100px"
                            />
                            <span v-else>-</span>
                        </td>
                        <td>
                            <a-select
                                v-model="item.cache_type"
                                placeholder="请选择"
                                @change="
                                    item.cache_type == 'all'
                                        ? (item.paths = [])
                                        : null
                                "
                                style="width: 120px"
                            >
                                <a-option
                                    label="全部文件"
                                    value="all"
                                ></a-option>
                                <a-option
                                    label="文件后缀"
                                    value="suffix"
                                ></a-option>
                                <a-option
                                    label="文件目录"
                                    value="dir"
                                ></a-option>
                            </a-select>
                        </td>
                        <td>
                            <span v-if="item.cache_type == 'all'"
                                >全部文件</span
                            >
                            <!-- <a-input v-else v-model="item.paths" placeholder="多个参数用“;”分割"></a-input> -->
                            <a-input-tag
                                v-else
                                v-model="item.paths"
                                @blur="(v) => inputTagBlur(v, item.paths)"
                                @change="(v) => (item.paths = checkPath(v))"
                                style="width: 240px"
                                placeholder="输入后回车，支持多个参数"
                            />
                        </td>
                        <td>
                            <a-input
                                v-model="item.weight"
                                type="number"
                                placeholder="权重"
                                style="width: 70px"
                            />
                        </td>
                        <td>
                            <a-switch v-model="item.enable_stream"></a-switch>
                        </td>
                        <td>
                            <span
                                class="c-blue cursor"
                                @click="path_cache_rules.splice(index, 1)"
                                >删除</span
                            >
                        </td>
                    </tr>
                    <tr>
                        <td
                            colspan="7"
                            class="cursor"
                            @click="
                                path_cache_rules.push({
                                    cache_ttl: '60',
                                    cache_type: 'all',
                                    enable: true,
                                    paths: [],
                                    weight: '1',
                                    enable_stream: false,
                                })
                            "
                            style="background: var(--color-neutral-1)"
                        >
                            <div class="df ai-c jc-c">
                                <icon-plus :size="14" class="c-99" />
                                <span class="c-99 lh-1" style="margin-left: 6px"
                                    >添加</span
                                >
                            </div>
                        </td>
                    </tr>
                </tbody>
            </table>

            <div class="b mt-40">缓存键规则配置</div>
            <div class="mt-4 fs-12 c-99">
                通过缓存键规则配置，可筛选影响资源内容的参数作为缓存键，将同类资源请求映射到同一缓存键，从而提升缓存命中率。
            </div>
            <table class="com-table mt-10">
                <tbody>
                    <tr class="thead">
                        <td>类型</td>
                        <td>路径</td>
                        <td>忽略大小写</td>
                        <td>忽略参数</td>
                        <td>参数</td>
                        <td>
                            <a-popover position="bottom">
                                <span class="cursor"
                                    >权重<icon-question-circle-fill
                                        class="c-99 ml-4"
                                /></span>
                                <a-tooltip content="权重值越小优先级越高">
                                    <el-icon
                                        class="cursor c-99"
                                        style="margin-left: 4px"
                                        ><WarningFilled
                                    /></el-icon>
                                </a-tooltip>
                            </a-popover>
                        </td>
                        <td>操作</td>
                    </tr>
                    <tr
                        v-for="(item, index) in path_key_cache_rules"
                        :key="index"
                        style="background: var(--color-neutral-1)"
                    >
                        <td>
                            <a-select
                                v-model="item.cache_type"
                                placeholder="请选择"
                                @change="
                                    item.cache_type == 'all'
                                        ? (item.paths = [])
                                        : null
                                "
                                style="width: 120px"
                            >
                                <a-option
                                    label="全部文件"
                                    value="all"
                                ></a-option>
                                <a-option
                                    label="文件后缀"
                                    value="suffix"
                                ></a-option>
                                <a-option
                                    label="文件目录"
                                    value="dir"
                                ></a-option>
                            </a-select>
                        </td>
                        <td>
                            <span v-if="item.cache_type == 'all'"
                                >全部文件</span
                            >
                            <!-- <a-input v-else v-model="item.paths" placeholder="多个参数用“;”分割"></a-input> -->
                            <a-input-tag
                                v-else
                                v-model="item.paths"
                                @blur="(v) => inputTagBlur(v, item.paths)"
                                @change="(v) => (item.paths = checkPath(v))"
                                style="width: 200px"
                                placeholder="输入后回车，支持多个参数"
                            />
                        </td>
                        <td>
                            <a-switch v-model="item.ignore_case"></a-switch>
                        </td>
                        <td>
                            <a-select
                                v-model="item.ignore_key_rule"
                                placeholder="请选择"
                                @change="
                                    item.ignore_key_rule == 'ignore' ||
                                    item.ignore_key_rule == 'keep'
                                        ? (item.keys = '')
                                        : null
                                "
                                style="width: 140px"
                            >
                                <a-option
                                    label="不忽略"
                                    value="keep"
                                ></a-option>
                                <a-option
                                    label="全部忽略"
                                    value="ignore"
                                ></a-option>
                                <a-option
                                    label="忽略指定参数"
                                    value="ignore_specified"
                                ></a-option>
                                <a-option
                                    label="保留指定参数"
                                    value="keep_specified"
                                ></a-option>
                            </a-select>
                        </td>
                        <td>
                            <span v-if="item.ignore_key_rule == 'keep'">-</span>
                            <span v-else-if="item.ignore_key_rule == 'ignore'"
                                >全部参数</span
                            >
                            <!-- <a-input v-else v-model="item.keys" placeholder="多个参数用“;”分割"></a-input> -->
                            <a-input-tag
                                v-else
                                v-model="item.keys"
                                @blur="(v) => inputTagBlur(v, item.keys)"
                                @change="(v) => (item.keys = checkPath(v))"
                                style="width: 200px"
                                placeholder="输入后回车，支持多个参数"
                            />
                        </td>
                        <td>
                            <a-input
                                v-model="item.weight"
                                type="number"
                                placeholder="权重"
                                style="width: 70px"
                            />
                        </td>
                        <td>
                            <span
                                class="c-blue cursor"
                                style="white-space: nowrap"
                                @click="path_key_cache_rules.splice(index, 1)"
                                >删除</span
                            >
                        </td>
                    </tr>
                    <tr style="background: var(--color-neutral-1)">
                        <td
                            colspan="7"
                            class="cursor"
                            @click="
                                path_key_cache_rules.push({
                                    cache_type: 'all',
                                    paths: [],
                                    keys: [],
                                    ignore_case: false,
                                    ignore_key_rule: 'keep',
                                    weight: '1',
                                })
                            "
                        >
                            <div class="df ai-c jc-c">
                                <icon-plus :size="14" class="c-99" />
                                <span class="c-99 lh-1" style="margin-left: 6px"
                                    >添加</span
                                >
                            </div>
                        </td>
                    </tr>
                </tbody>
            </table>

            <div class="b mt-40">源站信息配置</div>
            <div class="mt-10 padding-20" style="background:var(--color-neutral-1)">
                <el-form-item label="源站地址" label-width="80px" style="margin-bottom: 0">
                    <el-input v-model="endpoint.server_url_after" style="width:600px;">
                        <template #prepend>
                            <el-select
                                style="width:100px;"
                                placeholder="请选择"
                                v-model="endpoint.server_url_pre"
                            >
                                <el-option label="http://" value="http://"></el-option>
                                <el-option label="https://" value="https://"></el-option>
                            </el-select>
                        </template>
                    </el-input>
                </el-form-item>
            </div>
        </div>

        <div class="site-form-actions">
            <a-button type="primary" @click="submit">保存配置</a-button>
            <a-button v-if="inWujie" class="ml-20" @click="close">取消</a-button>
        </div>
    </div>
</template>

<script>
import myAxios from "../../utils/index";
import { getGlobalCacheRepository, responseData } from "../../api/config";
import CacheRepositoryForm from "../../components/cache-repository-form.vue";
import { IconPlus, IconQuestionCircleFill } from "@arco-design/web-vue/es/icon";
export default {
    name: "k8s-catch",
    components: {
        IconPlus,
        IconQuestionCircleFill,
        CacheRepositoryForm,
    },
    data() {
        return {
            tab: "2",
            path_cache_rules: [],
            path_key_cache_rules: [],
            newForm: {},
            storage_source: {},
            extra: {},
            inWujie: false,
            globalRepositoryLoading: true,
            globalRepository: {
                repository_url: "",
                storage_path: "/",
                username: "",
                password: "",
            },
            cacheRepository: {
                mode: "global",
                repository_url: "",
                storage_path: "/",
                username: "",
                password: "",
            },
            endpoint: {
                server_url_pre: "http://",
                server_url_after: "",
            },
        };
    },
    created() {
        this.inWujie = window?.__POWERED_BY_WUJIE__;
        this.loadGlobalRepository();

        myAxios
            .post("/api/setting/get", {
                group: this.$route.params.host,
                path_prefix: this.$route.query.path_prefix,
            })
            .then((res) => {
                let data = res.data.data;
                if (data) {
                    this.path_cache_rules = data.path_cache_rules || [];
                    this.path_key_cache_rules = data.path_key_cache_rules || [];
                    this.storage_source = data.storage_source || {
                        endpoint: "",
                    };
                    this.extra = data.extra || {};
                    const storedCacheRepository = this.extra.cache_repository || {};
                    this.cacheRepository = {
                        mode: ["global", "custom"].includes(storedCacheRepository.mode)
                            ? storedCacheRepository.mode
                            : "global",
                        repository_url: storedCacheRepository.repository_url || "",
                        storage_path: storedCacheRepository.storage_path || "/",
                        username: storedCacheRepository.username || "",
                        password: storedCacheRepository.password || "",
                    };
                    this.normalizeRepositoryPath();
                    this.endpoint = {
                        server_url_pre: this.storage_source?.endpoint?.match?.(/^(https?:\/\/)/)?.[0] || "http://",
                        server_url_after: this.storage_source?.endpoint?.replace?.(/^https?:\/\//,'') || "",
                    };
                    this.newForm = {
                        priority: "",
                        access_key: "",
                        secret_key: "",
                        bucket: "",
                        endpoint: "",
                        region: "",
                        cache_header: false,
                        cache_ttl: 300,
                        disabled: true,
                        rewrite_host: "",
                        ...data.minio,
                    };
                }
                // if (this.$route.query.endpoint) {
                //     this.storage_source = {
                //         endpoint: this.$route.query.endpoint,
                //         path_prefix: this.$route.query.path_prefix || "",
                //         rewrite_host: this.$route.query.rewrite_host || "",
                //         rewrite_path: this.$route.query.rewrite_path || "",
                //         path_match_type:
                //             this.$route.query.path_match_type || "prefix",
                //     };
                // }
            })
            .catch(() => {
                // 详情页保留默认表单，错误信息由 axios 拦截器统一处理。
            });
    },
    computed: {
        activeCacheRepository() {
            const repository = this.cacheRepository.mode === "global"
                ? this.globalRepository
                : this.cacheRepository;
            return {
                repository_url: repository.repository_url || "",
                storage_path: this.cacheRepository.storage_path,
                username: repository.username || "",
                password: repository.password || "",
            };
        },
    },
    methods: {
        async loadGlobalRepository() {
            this.globalRepositoryLoading = true;
            try {
                const response = await getGlobalCacheRepository(true);
                const repository = responseData(response);
                this.globalRepository = {
                    repository_url: repository.repository_url || "",
                    storage_path: repository.storage_path || "/",
                    username: repository.username || "",
                    password: repository.password || "",
                };
            } catch (e) {
                this.globalRepository = {
                    repository_url: "",
                    storage_path: "/",
                    username: "",
                    password: "",
                };
            } finally {
                this.globalRepositoryLoading = false;
            }
        },
        normalizeRepositoryPath() {
            const value = this.cacheRepository.storage_path?.trim() || "/";
            this.cacheRepository.storage_path = value.startsWith("/") ? value : `/${value}`;
        },
        updateCacheRepository(value) {
            this.cacheRepository = {
                ...this.cacheRepository,
                repository_url: this.cacheRepository.mode === "custom"
                    ? value.repository_url
                    : this.cacheRepository.repository_url,
                storage_path: value.storage_path,
                username: this.cacheRepository.mode === "custom"
                    ? value.username
                    : this.cacheRepository.username,
                password: this.cacheRepository.mode === "custom"
                    ? value.password
                    : this.cacheRepository.password,
            };
        },
        validateRepository() {
            this.normalizeRepositoryPath();
            if (this.cacheRepository.mode === "global" && !this.globalRepository.repository_url) {
                this.$message.warning("请先配置全局缓存仓库");
                return false;
            }
            if (this.cacheRepository.mode === "custom" && !this.cacheRepository.repository_url) {
                this.$message.warning("请输入镜像仓库地址");
                return false;
            }
            return true;
        },
        inputTagBlur(v, obj) {
            let value = v.target.value.replace(/^\/+/, "");
            value && obj?.push(value);
        },
        checkPath(v) {
            return v.map((i) => i.replace(/^\/+/, "")).filter((i) => i);
        },
        submit() {
            if (!this.validateRepository()) return;
            // 将cache_ttl，weight字段转化为数字
            this.path_cache_rules.forEach((item) => {
                item.cache_ttl = parseInt(item.cache_ttl);
                item.weight = parseInt(item.weight);
            });
            this.path_key_cache_rules.forEach((item) => {
                item.weight = parseInt(item.weight);
            });
            myAxios
                .post("/api/setting/set", {
                    group: this.$route.params.host,
                    storage_source: {
                        ...this.storage_source,
                        endpoint: this.endpoint.server_url_pre + this.endpoint.server_url_after,
                    },
                    minio: this.newForm,
                    path_cache_rules: this.path_cache_rules,
                    path_key_cache_rules: this.path_key_cache_rules,
                    extra: {
                        ...this.extra,
                        ingress_name: this.$route.query.ingress_name || "",
                        cache_repository: {
                            mode: this.cacheRepository.mode,
                            repository_url: this.cacheRepository.mode === "custom"
                                ? this.cacheRepository.repository_url
                                : "",
                            storage_path: this.cacheRepository.storage_path,
                            username: this.cacheRepository.mode === "custom"
                                ? this.cacheRepository.username
                                : "",
                            password: this.cacheRepository.mode === "custom"
                                ? this.cacheRepository.password
                                : "",
                        },
                    },
                })
                .then(() => {
                    this.$message.success("操作成功");
                });
        },
        close() {
            if (window?.__POWERED_BY_WUJIE__) {
                window.$wujie?.bus.$emit("close");
            }
        },
    },
};
</script>

<style>
.repository-toolbar {
    display: flex;
    margin-bottom: 20px;
    align-items: center;
}

.site-form-actions {
    display: flex;
    padding: 20px 0;
    align-items: center;
}

.file-cache-tabs .arco-tabs-nav-type-line .arco-tabs-tab {
    margin-left: 0;
    margin-right: 32px;
}
</style>
