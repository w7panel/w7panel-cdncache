<template>
    <div class="padding-20">
        <el-alert
            v-if="loadError"
            title="全局存储配置加载失败，可以稍后重试"
            type="warning"
            show-icon
            :closable="false"
            class="mb-20"
        />

        <el-skeleton v-if="loading" :rows="5" animated />
        <template v-else>
            <StorageConfigForm v-model="form" />
            <div class="form-actions">
                <el-button type="primary" :loading="saving" @click="submit">
                    保存配置
                </el-button>
            </div>
        </template>
    </div>
</template>

<script>
import {
    getGlobalStorageConfig,
    responseData,
    saveGlobalStorageConfig,
} from "../../api/config";
import StorageConfigForm from "../../components/storage-config-form.vue";

const createForm = () => ({
    access_key: "",
    secret_key: "",
    bucket: "",
    endpoint: "",
    region: "",
});

export default {
    name: "GlobalStorageSetting",
    components: {
        StorageConfigForm,
    },
    data() {
        return {
            form: createForm(),
            loading: true,
            saving: false,
            loadError: false,
        };
    },
    created() {
        this.load();
    },
    methods: {
        async load() {
            this.loading = true;
            this.loadError = false;
            try {
                const response = await getGlobalStorageConfig(true);
                this.form = {
                    ...createForm(),
                    ...responseData(response),
                };
            } catch (e) {
                this.loadError = true;
            } finally {
                this.loading = false;
            }
        },
        async submit() {
            if (!this.form.endpoint || !this.form.bucket) {
                this.$message.warning("请输入 S3 host 和 bucket");
                return;
            }

            this.saving = true;
            try {
                await saveGlobalStorageConfig(this.form);
                this.$message.success("全局存储配置已保存");
                this.loadError = false;
            } catch (e) {
                this.$message.error("保存失败，请稍后重试");
            } finally {
                this.saving = false;
            }
        },
    },
};
</script>

<style scoped>
.form-actions {
    display: flex;
    max-width: 760px;
    margin-top: 32px;
    justify-content: flex-start;
}
</style>
