//
// Licensed to the Apache Software Foundation (ASF) under one or more
// contributor license agreements.  See the NOTICE file distributed with
// this work for additional information regarding copyright ownership.
// The ASF licenses this file to You under the Apache License, Version 2.0
// (the "License"); you may not use this file except in compliance with
// the License.  You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

#[link(wasm_import_module = "sharding")]
extern "C" {
    fn poll_table(addr: i64, len: i32) -> i32;
}

// The value of sharding_count must be consistent with the value of the AvaliableTargetNames
const SHARDING_COUNT: u8 = 3;

#[no_mangle]
pub unsafe extern "C" fn do_work() -> i64 {
    let mut buf = [0u8; 1024];
    let buf_ptr = buf.as_mut_ptr() as i64;
    let len = poll_table(buf_ptr, buf.len() as i32);
    let parts = std::slice::from_raw_parts(buf_ptr as *const u8, len as usize);
    let target_names_length = parts[0] as usize;
    //let raw_str = String::from_utf8_lossy(&parts[1..1+ target_names_length]);
    //let target_names = raw_str.split(',').collect::<Vec<&str>>();

    let condition_length = parts[1+ target_names_length] as usize;
    let raw_str = String::from_utf8_lossy(&parts[2 + target_names_length .. 2 + target_names_length + condition_length]);
    let condition_values = raw_str.split(',').collect::<Vec<&str>>();
    let column_value = condition_values[2].parse::<u8>().unwrap();
    let sharding =  column_value % SHARDING_COUNT;
    let mut table_name = format!("{}_{}", condition_values[1], sharding);
    //buf_slice.append(table_name.as_mut_vec());
    std::ptr::copy_nonoverlapping(table_name.as_mut_ptr() as *const _, buf.as_mut_ptr().add(len as usize), table_name.len());
    buf_ptr
}
