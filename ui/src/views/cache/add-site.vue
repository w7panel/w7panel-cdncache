<template>
    <el-dialog
        :model-value="modelValue"
        title="添加站点"
        width="800px"
        @update:model-value="$emit('update:modelValue', $event)"
    >
        <DomainParseAlert />

        <el-form label-width="auto" class="padding-20">
            <el-form-item label="域名">
                <div class="domain-list-wrapper">
                    <div v-for="(item, index) in form.domains" :key="index" class="domain-item">
                        <el-input
                            v-model="item.url_after"
                            placeholder="请输入域名"
                            class="domain-input"
                        >
                            <template #prepend>
                                <span>{{ item.auto_https ? "https://" : "http://" }}</span>
                            </template>
                        </el-input>

                        <el-checkbox v-model="item.auto_https" class="auto-https-checkbox">
                            自动https
                        </el-checkbox>

                        <el-icon
                            v-if="form.domains.length > 1"
                            class="delete-icon"
                            @click="removeDomain(index)"
                        >
                            <Delete />
                        </el-icon>
                    </div>

                    <el-button type="primary" plain class="add-domain-btn" @click="addDomain">
                        <el-icon><Plus /></el-icon>
                        添加域名
                    </el-button>
                </div>
            </el-form-item>
        </el-form>

        <template #footer>
            <div class="dialog-footer">
                <el-button @click="handleCancel">取消</el-button>
                <el-button type="primary" :loading="loading" @click="handleConfirm">确定</el-button>
            </div>
        </template>
    </el-dialog>
</template>

<script>
import { ElMessage } from "element-plus";
import { Delete, Plus } from "@element-plus/icons-vue";
import DomainParseAlert from "./domain-parse-alert.vue";

export default {
    name: "AddSiteDialog",
    components: {
        DomainParseAlert,
        Plus,
        Delete,
    },
    props: {
        modelValue: {
            type: Boolean,
            default: false,
        },
        data: {
            type: Object,
            default: () => ({}),
        },
    },
    emits: ["update:modelValue", "confirm"],
    data() {
        return {
            form: {
                domains: [],
            },
            loading: false,
        };
    },
    watch: {
        data: {
            handler(newVal) {
                if (!newVal) return;
                this.form = {
                    ...this.form,
                    ...newVal,
                };
                if (!this.form.domains || this.form.domains.length === 0) {
                    this.form.domains = [
                        {
                            url_after: "",
                            auto_https: true,
                        },
                    ];
                }
            },
            immediate: true,
            deep: true,
        },
        modelValue(v){
            if(!v){this.loading = false;}
        }
    },
    methods: {
        addDomain() {
            this.form.domains.push({
                url_after: "",
                auto_https: true,
            });
        },
        removeDomain(index) {
            this.form.domains.splice(index, 1);
        },
        validateDomains() {
            for (let i = 0; i < this.form.domains.length; i++) {
                const domain = this.form.domains[i].url_after.trim();
                if (!domain) {
                    ElMessage.warning(`请填写第 ${i + 1} 个域名`);
                    return false;
                }
                const domainRegex = /^(?!:\/\/)([a-zA-Z0-9-]+\.)+[a-zA-Z]{2,}(:\d+)?$/;
                if (!domainRegex.test(domain)) {
                    ElMessage.warning(`第 ${i + 1} 个域名格式不正确`);
                    return false;
                }
            }
            return true;
        },
        handleConfirm() {
            if (!this.validateDomains()) return;
            this.loading = true;
            this.$emit("confirm", this.form);
            // this.$emit("update:modelValue", false);
        },
        handleCancel() {
            this.$emit("update:modelValue", false);
        },
    },
};
</script>

<style scoped>
.padding-20 {
    padding: 20px;
}

.domain-list-wrapper {
    display: flex;
    flex: 1;
    flex-direction: column;
}

.domain-item {
    display: flex;
    align-items: center;
    margin-bottom: 10px;
}

.domain-input {
    flex: 1;
}

.auto-https-checkbox {
    margin-left: 10px;
    flex-shrink: 0;
}

.delete-icon {
    margin-left: 10px;
    color: #f56c6c;
    cursor: pointer;
    flex-shrink: 0;
    font-size: 18px;
}

.delete-icon:hover {
    color: #ff4d4f;
}

.add-domain-btn {
    width: 100%;
    margin-top: 10px;
}

.dialog-footer {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 8px;
}
</style>
