<template>
    <div>
        <el-alert
            v-if="domainParse.exist"
            title="添加域名后，请在您持有域名的DNS解析后台添加对应的域名解析记录："
            type="info"
            show-icon
            :closable="false"
            class="alert-style"
        >
            <span class="alert-style-default">
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

.alert-style{
    background-color: rgb(232, 243, 255)!important;
    padding: 16px!important;
    align-items: start!important;
}
.alert-style.el-alert .el-alert__icon{
    font-size: 16px!important;
    margin-right: 8px!important;
    height: 22px!important;
    width: 22px!important;
    color: rgb(22,93,255)!important;
}
.alert-style .el-alert__title{
    margin-bottom: 4px;
    font-weight: 500;
    font-size: 16px;
    line-height: 1.5;
    color: rgb(29,33,41);
}
.alert-style .el-alert__content{padding-left:0!important;}
.alert-style-ul{
    color: rgb(78,89,105)!important;
    margin:0;
    line-height: 22px;
    padding-inline-start: 18px;
    font-size:14px!important;
}
.alert-style-default{
    color: rgb(78,89,105)!important;
    font-size:14px!important;
}
</style>
