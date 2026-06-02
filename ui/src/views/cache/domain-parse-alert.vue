<template>
    <div>
        <el-alert
            v-if="domainParse.exist"
            title="添加域名后，请在您持有域名的DNS解析后台添加对应的域名解析记录："
            type="info"
            show-icon
            :closable="false"
            class="domain-parse-alert"
        >
            <span style="color:var(--el-color-primary)">
                <span>记录类型：</span>
                <span>{{ domainParse.type }}；</span>
                <span>记录值：</span>
                <span>{{ domainParse.type === "A" ? domainParse.ips : domainParse.cname }}</span>
                <span v-if="domainParse.type === 'A' && domainParse.ips.includes(',')">
                    （IP任选一个，解析功能也支持添加多条记录）
                </span>
            </span>
        </el-alert>
    </div>
</template>
<script>
// import axios from "axios";
import myAxios from '@/utils';

export default {
    name: "DomainParseAlert",
    data() {
        return {
            domainParse: {
                exist: false,
                type: "A",
                cname: "",
                ips: "",
            },
        };
    },
    mounted() {
        this.init();
    },
    methods: {
        init() {
            const namespaceActive = "default";
            myAxios
                .get(`/k8s-proxy/api/v1/namespaces/${namespaceActive}/configmaps/domain-parse`, {
                    baseURL: "",
                    loading: true,
                })
                .then((res) => {
                    this.domainParse = {
                        exist: true,
                        type: res.data?.data?.type || "A",
                        cname: res.data?.data?.cname || "",
                        ips: res.data?.data?.ips || "",
                    };
                })
                .catch(() => {});
        },
    },
};
</script>
<style>
.domain-parse-alert.el-alert{
    background-color: var(--el-color-primary-light-9);
    color: var(--el-color-primary);
}
</style>
