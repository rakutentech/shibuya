This is a very simple http based file storage program. Only meant to be used for local testing and demo purpose of Shibuya. **Do not use in production**

# Usage

1. GET    /folder_name/folder_name/file_name fetches the file if present in the path
2. PUT    /folder_name/folder_name/file_name stores the attached file in path with the file_name. Overwrites if already exists
3. DELETE /folder_name/folder_name/file_name deletes the file if exists