import axios from "axios";
import { ElMessage,ElMessageBox } from 'element-plus';

const myAxios = axios.create({
    // baseURL: window.location.protocol+'//console.w7.cc/',
    baseURL: process.env.NODE_ENV === 'production' ? window?.$wujie?.props?.url : '',
    // 测试
    // baseURL: '/panel-api/v1/microapp/w7-cdncache-qpiuzwnx/proxy',
    timeout: 90000
});

// myAxios.getToken = (callback)=>{
//     if (window.__MICRO_APP_ENVIRONMENT__ ) {
//         let data = window.microApp.getData();
//
//         data.w7.login().then((code) => {
//             axios.post('/api/thirdparty-cd/login', {
//                 code: code.code
//             }).then(res => {
//                 window.formrelogining = false;
//                 data.w7.setStorage({key:"token",value:res.data.access_token});
//                 data.w7.setStorage({key:'nickname', value:res.data.nickname});
//                 callback && callback();
//             }).catch(()=>{
//                 window.formrelogining = false;
//             });
//         })
//     } else {
//         let code = localStorage.getItem("save-code");
//         let host = window?.$wujie?.props?.url.replace(/\/$/,'') || '';
//         axios.post(host + '/api/thirdparty-cd/login', {code}).then(res => {
//             window.formrelogining = false;
//             if(res?.data?.access_token){
//                 localStorage.setItem("token",res.data.access_token);
//                 sessionStorage.setItem('nickname',res.data.nickname);
//                 callback && callback();
//             }
//         }).catch(()=>{
//             window.formrelogining = false;
//         });
//     }
// }

myAxios.interceptors.request.use(config => {
    // let token = null;
    // if (window.__MICRO_APP_ENVIRONMENT__ ) {
    //     let data = window.microApp.getData();
    //     token = data.w7.getStorage("token");
    // }else{
    //     // token = '0pJLy1I2EgxhSNRt'
    //     token = localStorage.getItem("token");
    // }
    // if(token){
    //     config.headers.Authorization = 'Bearer ' + token
    // }
    // config.headers.Authorization = 'bearer ' + (window.$wujie?.props?.OAUTH_TOKEN ?? 'xdzsdounrc')
    config.headers.authorizationx = 'bearer ' + window.$wujie?.props?.paneltoken;
    // 测试
    // config.headers.authorizationx = 'bearer ' + 'eyJhbGciOiJSUzI1NiIsImtpZCI6IkRUc1FoNVR0Q1Z6SDBmc1dMZkxUTmd0MEtZSTlpajZzNXJkX3hrWHloaTQifQ.eyJhdWQiOlsiYWRtaW4iLCJmb3VuZGVyIiwiNDkwMjA5IiwiaHR0cHM6Ly9rdWJlcm5ldGVzLmRlZmF1bHQuc3ZjLmNsdXN0ZXIubG9jYWwiLCJrM3MiXSwiZXhwIjoxNzc4Njc0MzE3LCJpYXQiOjE3Nzg2MzgzMTcsImlzcyI6Imh0dHBzOi8va3ViZXJuZXRlcy5kZWZhdWx0LnN2Yy5jbHVzdGVyLmxvY2FsIiwianRpIjoiOWIzNjE1YWEtZjZiNi00MGIzLThkMDctZGEwOGJmZTc4MjZiIiwia3ViZXJuZXRlcy5pbyI6eyJuYW1lc3BhY2UiOiJkZWZhdWx0Iiwic2VydmljZWFjY291bnQiOnsibmFtZSI6ImFkbWluIiwidWlkIjoiMDY4ZTFhOWEtMjE3Yy00NDBlLThjZGUtNTVhMzFkNzQ2YjcyIn19LCJuYmYiOjE3Nzg2MzgzMTcsInN1YiI6InN5c3RlbTpzZXJ2aWNlYWNjb3VudDpkZWZhdWx0OmFkbWluIn0.x5EdHB4ziyFwSFNJU--WYLxDgAeHecrxLqmjEhKMromSZPJA_pCFpOD-bFk1t4aUVG0XZctwgprg1lzKX1s2n8cyhYSRSF7AIvC64Xld5fWG71pUZTMdLlMI05MvHN_YAlyDNrwa-PshBDTC6JPs7qCMTwqQQcwQz-s7ES5i7qI4FxyzORRN5RJJqQ34bZwh4wmHORVzEghUBU0j4KJjTfN-9P2gLIswV6tx1cTovgXy6AHhGTOSwDg8GSQ6KAebwYGR3G7jKVYHjdPefqvCn7v6dVZ2hZoE29d7WnYbbPjkfIvxJz-F8ARyYRsWZwjJhhOTWbEQk5hAQousYkZ20Q';
    console.log(config)
    return config
}, err => {
    Promise.reject(err)
})

myAxios.interceptors.response.use(res => {
    if(res.status>=200 && res.status<300 && res){
        return Promise.resolve(res)
    }
}, error => {
    if(error?.response?.status == 401){
        if(window.formrelogining){return}
        window.formrelogining = true;
        return;
    }

    if(error?.response?.status == 422){
        let errorinfo = error.response.data.errors;
        if(!errorinfo){return}
        let keys = Object.keys(errorinfo);
        let messages = keys.map(key => {
            return errorinfo[key].join('\n');
        });

        ElMessageBox.alert(messages.join('<br/>'),"提示",{confirmButtonText:"确定",dangerouslyUseHTMLString:true});
        
        return Promise.reject(error);
    }
    
    if(error?.response?.status == 429){return}
    if(error?.response?.status == 408){return}
    if(!error?.config?.dontalert){
        if(error.response && !error.config.headers.cancelerror && error.response?.data?.error) {
            ElMessage({
                message: error.response.data.error,
                duration: 3000,
                type:'error',
            });
        }
    }

    return Promise.reject(error)
});

export default myAxios;
